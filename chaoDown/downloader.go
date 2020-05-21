package chaoDown

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bzsome/ChaoGoDown/utils"
	"github.com/bzsome/chaoGo/workpool"

	"github.com/dustin/go-humanize"
	"github.com/shopspring/decimal"
)

//默认的下载配置
var DefaultDownloader = Downloader{
	Path:      "downloads",
	PoolSize:  50,
	ChuckSize: 1024 * 100,
}

type Downloader struct {
	Path      string //保存文件路径
	FileName  string //保存文件名
	PoolSize  int    //线程池大小
	ChuckSize int64  //每个线程池下载块大小

	request      *Request
	osFile       *os.File // 保存至本地文件的file对象
	fileFullName string   // 文件夹+文件名
	configFile   string   // 配置文件名

	chunkIndex int64      //下载块序号
	chunkMutex sync.Mutex //线程锁
	statTime   time.Time  //下载开始时间
	endTime    time.Time  //下载结束时间
}

// 返回文件的相关信息
func (down *Downloader) Resolve() error {
	httpRequest, err := BuildHTTPRequest(down.request)
	if err != nil {
		return err
	}

	// Use "Range" header to resolve  请求长度为0的判断，以便获得文件信息
	httpRequest.Header.Add("Range", "bytes=0-0")
	httpClient := BuildHTTPClient()
	response, err := httpClient.Do(httpRequest)
	if err != nil {
		return err
	}

	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return fmt.Errorf("response status error:%d", response.StatusCode)
	}

	//  Get file name by "Content-Disposition" 从 "Content-Disposition" 中获得文件名
	contentDisposition := response.Header.Get("Content-Disposition")
	if contentDisposition != "" {
		_, params, _ := mime.ParseMediaType(contentDisposition)
		filename := params["filename"]
		if filename != "" {
			down.request.fileName = filename
		}
	}

	// Is support range 支持分段下载
	down.request.Range = response.StatusCode == 206
	// Get file size 获得文件大小
	if down.request.Range {
		contentRange := response.Header.Get("Content-Range")
		if contentRange != "" {
			// e.g. bytes 0-1000/1001 => 1001
			total := subLastSlash(contentRange)
			if total != "" && total != "*" {
				parse, err := strconv.ParseInt(total, 10, 64)
				if err != nil {
					return err
				}
				down.request.fileSize = parse
			}
		}
	} else {
		contentLength := response.Header.Get("Content-Length")
		if contentLength != "" {
			down.request.fileSize, _ = strconv.ParseInt(contentLength, 10, 64)
		}
	}
	return nil
}

//初始化，分析url，获得文件长度；从配置文件中读取已下载块
func (down *Downloader) init(request *Request) error {
	down.request = request

	defer fmt.Println()

	fmt.Print("->1.初始化用户配置")
	if !strings.HasPrefix(request.URL, "http") {
		return errors.New("url不能为空，" + request.URL)
	}

	//创建下载目录文件夹
	if _, err := os.Stat(down.Path); os.IsNotExist(err) {
		if err := os.Mkdir(down.Path, os.ModePerm); err != nil {
			return err
		}
	}

	//设置默认值(没有指定的配置，从默认配置中读取)
	utils.CopyValue2(down, &DefaultDownloader, utils.EmpValue)

	fmt.Print("->2.读取服务器信息")

	//获得文件大小信息
	if err := down.Resolve(); err != nil {
		return err
	}

	if down.FileName == "" {
		down.FileName = down.request.fileName
	}
	if down.FileName == "" {
		down.FileName = path.Base(request.URL)
	}
	down.fileFullName = path.Join(down.Path, down.FileName)
	file, err := os.OpenFile(down.fileFullName, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return err
	} else {
		down.osFile = file
	}

	fmt.Print("->3.读取数据块信息")
	if down.ChuckSize <= 0 {
		return errors.New("ChuckSize大小不能为0")
	} else {
		if err = down.initSubs(); err != nil {
			return err
		}
	}

	return nil
}

//初始化下载进度(首先重文件中读取已下载完成的片段)
func (down *Downloader) initSubs() error {
	if down.request.fileSize <= 0 {
		return errors.New("无法获得文件大小")
	}

	down.configFile = down.fileFullName + ".yaml"
	utils.GetConfigYaml(down.configFile, down.request)
	down.request.Subeds = mergeSub(down.request.Subeds)

	//构造完整的片段
	allSubs, _ := down.generateSubs()

	//判断此段是否已完全下载
	isDowned := func(one [2]int64, Subeds [][2]int64) bool {
		//已下载的必须完全包含此段
		for _, sed := range Subeds {
			if sed[0] <= one[0] && sed[1] >= one[1] {
				return true
			}
		}
		return false
	}

	tempSubeds := [][2]int64{}
	for _, one := range allSubs {
		downed := isDowned(one, down.request.Subeds)
		if !downed {
			tempSubeds = append(tempSubeds, one)
		}
	}
	down.request.unSubs = tempSubeds
	return nil
}

// Down
//支持分段下载，且程序中断重启能够继续下载
func (down *Downloader) Down(request *Request) error {
	//初始化下载文件信息
	err := down.init(request)
	if err != nil {
		return err
	}

	defer down.osFile.Close()

	fmt.Println("->开始下载")
	//创建线程池下载文件
	down.statTime = time.Now()

	wp := workpool.New(down.PoolSize)
	for _, oneChuck := range down.request.unSubs {
		//注意闭包
		doOneChuck := down.doOneChuck(oneChuck)
		wp.Do(doOneChuck)
	}
	wp.Wait()

	down.endTime = time.Now()

	if len(down.request.Subeds) == 1 {
		fmt.Println("OK，下载完成！")
	} else {
		fmt.Println("ERR，部分片段失败，请重试！")
	}
	return nil
}

func (down *Downloader) doOneChuck(one [2]int64) func() error {
	return func() error {
		done := down.downDone(one)
		down.downChunk(one[0], one[1], done)
		return nil
	}
}

//下载完成回调
func (down *Downloader) downDone(one [2]int64) func(err error) {
	done := func(err error) {
		if err != nil {
			fmt.Printf("down err %10s %10s %s\n", humanize.Bytes(uint64(one[0])), humanize.Bytes(uint64(one[1])), err)
		} else {
			down.chunkMutex.Lock()
			downDone(down, one)
			down.chunkMutex.Unlock()
		}
	}
	return done
}

//下载完成回调
func downDone(down *Downloader, one [2]int64) {
	request := down.request

	request.Subeds = append(request.Subeds, one)
	request.unSubs = DeleteSliceObject(request.Subeds, one)
	request.Subeds = mergeSub(request.Subeds)

	utils.WriteConfigYaml(down.configFile, request)

	//先处理下载进度，后打印(否则不精准)
	printRate(request)
}

//构造完整下载的片段
func (down *Downloader) generateSubs() ([][2]int64, error) {
	var subs [][2]int64

	// 支持断点续传
	if down.request.Range {
		chunkStart := int64(0)
		for {
			end := chunkStart + down.ChuckSize
			if end >= down.request.fileSize {
				end = down.request.fileSize
			}
			one := [2]int64{chunkStart, end}
			subs = append(subs, one)

			if end >= down.request.fileSize {
				break
			}
			chunkStart = chunkStart + down.ChuckSize
		}
	} else {
		//不支持断点续传，一次性下载全部
		subs = [][2]int64{{0, down.request.fileSize}}
	}
	return subs, nil
}

func subLastSlash(str string) string {
	index := strings.LastIndex(str, "/")
	if index != -1 {
		return str[index+1:]
	}
	return ""
}

func BuildHTTPRequest(request *Request) (*http.Request, error) {
	// Build request
	httpRequest, err := http.NewRequest(strings.ToUpper(request.Method), request.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range request.Header {
		httpRequest.Header.Add(k, v)
	}
	return httpRequest, nil
}

func BuildHTTPClient() *http.Client {
	// Cookie handle
	jar, _ := cookiejar.New(nil)

	return &http.Client{Jar: jar}
}

//分段下载，指定下载的起始
func (down *Downloader) downChunk(start int64, end int64, done func(err error)) {
	if done == nil {
		done = func(err error) {}
	}
	down.chunkMutex.Lock()
	down.chunkIndex = down.chunkIndex + 1
	fmt.Printf("chunk[%3d]   start - end   %10s -%10s\n", down.chunkIndex, humanize.Bytes(uint64(start)), humanize.Bytes(uint64(end)))
	down.chunkMutex.Unlock()

	httpRequest, _ := BuildHTTPRequest(down.request)
	httpRequest.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	httpClient := BuildHTTPClient()
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		done(err)
		return
	}
	defer httpResponse.Body.Close()
	buf := make([]byte, 8192)
	writeIndex := start
	for {
		n, err := httpResponse.Body.Read(buf)
		if n > 0 {
			writeSize, err := down.osFile.WriteAt(buf[0:n], writeIndex)
			if err != nil {
				done(err)
				return
			}
			writeIndex += int64(writeSize)
		}
		if err != nil {
			if err != io.EOF {
				done(err)
				return
			}
			done(nil)
			break
		}
	}
}

//打印下载进度
func printRate(request *Request) {
	fileSize := request.fileSize
	total := getDownTotal(request)

	decimal.DivisionPrecision = 2
	ds := decimal.NewFromInt(total * 100)
	fmt.Printf("\r%s", strings.Repeat(" ", 35))
	rate := ds.Div(decimal.NewFromFloat(float64(fileSize)))

	fmt.Printf("\rDownloading...%s %% \t (%s/%s) complete\n", rate, humanize.Bytes(uint64(total)), humanize.Bytes(uint64(fileSize)))
}

//删除指定的对象
func DeleteSliceObject(list [][2]int64, one [2]int64) [][2]int64 {
	ret := make([][2]int64, 0, len(list))
	for _, val := range list {
		if val[0] == one[0] && val[1] == one[1] {
			ret = append(ret, val)
		}
	}
	return ret
}

//根据下标删除
func DeleteSliceIndex(list [][2]int64, index int) [][2]int64 {
	return append(list[:index], list[index+1:]...)
}

//获得已下载号段的总长度
func getDownTotal(request *Request) int64 {
	var total = int64(0)
	for _, one := range request.Subeds {
		size := one[1] - one[0]
		total = total + size
	}
	return total
}

//合并连续的号段
func mergeSub(subs [][2]int64) [][2]int64 {
	//首先排序
	sortSub(subs)
	//如果结束大于等于 后面 的开始，则合并
	for i := 0; i < len(subs)-1; i++ {
		if subs[i][1] >= subs[i+1][0] {
			subs[i][1] = subs[i+1][1]
			subs = DeleteSliceIndex(subs, i+1)
			i = i - 1
		}
	}
	return subs
}

//对号段排序
func sortSub(subs [][2]int64) {
	sort.Slice(subs, func(i, j int) bool {
		return (subs)[i][0] < (subs)[j][0]
	})
}

//构造完整下载的片段
func (down *Downloader) GetExeTime() time.Duration {
	return down.endTime.Sub(down.statTime)
}

func (down *Downloader) GetSavePath() string {
	return down.fileFullName
}