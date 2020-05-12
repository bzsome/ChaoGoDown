package chaoHttp

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"ChaoGoDown/yamlConfig"
	"github.com/bzsome/chaoGo/workpool"
	"github.com/dustin/go-humanize"
	"github.com/shopspring/decimal"
)

type Downloader struct {
	Path     string
	FileName string
	//线程池大小
	PoolSize int
	//每个线程池下载块大小
	ChuckSize int64

	request      *Request
	response     *Response
	file         *os.File
	fileFullName string
	configFile   string
	doneChan     chan [2]int64
	index        int64
	mutex        sync.Mutex
}

// 返回文件的相关信息
func (down *Downloader) Resolve() (*Response, error) {
	httpRequest, err := BuildHTTPRequest(down.request)
	if err != nil {
		return nil, err
	}
	// Use "Range" header to resolve
	httpRequest.Header.Add("Range", "bytes=0-0")
	httpClient := BuildHTTPClient()
	response, err := httpClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return nil, fmt.Errorf("response status error:%d", response.StatusCode)
	}
	ret := &Response{}
	// Get file name by "Content-Disposition"
	contentDisposition := response.Header.Get("Content-Disposition")
	if contentDisposition != "" {
		_, params, _ := mime.ParseMediaType(contentDisposition)
		filename := params["filename"]
		if filename != "" {
			ret.Name = filename
		}
	}
	// Get file name by URL
	if ret.Name == "" {
		parse, err := url.Parse(httpRequest.URL.String())
		if err == nil {
			// e.g. /files/test.txt => test.txt
			ret.Name = subLastSlash(parse.Path)
		}
	}
	// Unknow file name
	if ret.Name == "" {
		ret.Name = "unknow"
	}
	// Is support range
	ret.Range = response.StatusCode == 206
	// Get file size
	if ret.Range {
		contentRange := response.Header.Get("Content-Range")
		if contentRange != "" {
			// e.g. bytes 0-1000/1001 => 1001
			total := subLastSlash(contentRange)
			if total != "" && total != "*" {
				parse, err := strconv.ParseInt(total, 10, 64)
				if err != nil {
					return nil, err
				}
				ret.Size = parse
			}
		}
	} else {
		contentLength := response.Header.Get("Content-Length")
		if contentLength != "" {
			ret.Size, _ = strconv.ParseInt(contentLength, 10, 64)
		}
	}
	return ret, nil
}
func (down *Downloader) Init(request *Request) error {
	defer fmt.Println()
	fmt.Print("初始化:")

	fmt.Print("->从服务器获取文件信息")
	down.request = request
	if down.FileName == "" {
		down.FileName = path.Base(request.URL)
	}

	if len(down.request.URL) <= 3 {
		return errors.New("url不能为空")
	}

	//创建下载目录文件夹
	if _, err := os.Stat(down.Path); os.IsNotExist(err) {
		err := os.Mkdir(down.Path, os.ModePerm)
		if err != nil {
			return err
		}
	}

	down.fileFullName = path.Join(down.Path, down.FileName)
	file, err := os.OpenFile(down.fileFullName, os.O_RDWR|os.O_CREATE, 0777)

	if err != nil {
		return err
	}
	down.file = file
	down.request = request
	down.response, err = down.Resolve()
	if err != nil {
		return err
	}

	fmt.Print("->读取数据块信息")
	down.request.fileSize = down.response.Size
	if down.ChuckSize <= 0 {
		return errors.New("subSize大小不能为0")
	}
	err = down.initSubs()
	if err != nil {
		return err
	}

	fmt.Println("->初始化完成")
	return nil
}

//初始化下载进度(首先重文件中读取已下载完成的片段)
func (down *Downloader) initSubs() error {
	if down.request.fileSize <= 0 {
		return errors.New("文件大小异常")
	}

	down.configFile = down.fileFullName + ".yaml"
	yamlConfig.GetConfigYaml(down.configFile, down.request)
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
func (down *Downloader) Down() error {
	defer down.file.Close()

	wp := workpool.New(down.PoolSize)
	for _, one := range down.request.unSubs {
		//注意闭包
		fn := down.doOneChuck(one)
		wp.Do(fn)
	}
	wp.Wait()
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
			down.mutex.Lock()
			downDone(down, one)
			down.mutex.Unlock()
		}
	}
	return done
}

//下载完成回调
func downDone(down *Downloader, one [2]int64) {
	request := down.request
	printRate(request)

	request.Subeds = append(request.Subeds, [2]int64{one[0], one[1]})
	request.unSubs = DeleteSlice(request.Subeds, one)
	request.Subeds = mergeSub(request.Subeds)

	yamlConfig.WriteConfigYaml(down.configFile, request)
}

//构造完整下载的片段
func (down *Downloader) generateSubs() ([][2]int64, error) {
	var subs [][2]int64

	// 支持断点续传
	response := down.response
	if response.Range {
		chunkStart := int64(0)
		for {
			end := chunkStart + down.ChuckSize
			if end >= response.Size {
				end = response.Size
			}
			one := [2]int64{chunkStart, end}
			subs = append(subs, one)

			if end >= response.Size {
				break
			}
			chunkStart = chunkStart + down.ChuckSize
		}
	} else {
		fmt.Println("不支持断点续传，一次性全部下载")
		subs = [][2]int64{{0, response.Size}}
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
	down.mutex.Lock()
	down.index = down.index + 1
	down.mutex.Unlock()
	fmt.Printf("chunk[%3d]   start - end   %10s -%10s\n", down.index, humanize.Bytes(uint64(start)), humanize.Bytes(uint64(end)))

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
			writeSize, err := down.file.WriteAt(buf[0:n], writeIndex)
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
func DeleteSlice(list [][2]int64, one [2]int64) [][2]int64 {
	ret := make([][2]int64, 0, len(list))
	for _, val := range list {
		if val[0] == one[0] && val[1] == one[1] {
			ret = append(ret, val)
		}
	}
	return ret
}

//根据下标删除
func DeleteSlice2(list [][2]int64, index int) [][2]int64 {
	return append(list[:index], list[index+1:]...)
}

func getDownTotal(request *Request) int64 {
	var total = int64(0)
	for _, one := range request.Subeds {
		size := one[1] - one[0]
		total = total + size
	}
	return total
}

//对号段排序
func sortSub(subs [][2]int64) {
	sort.Slice(subs, func(i, j int) bool {
		return (subs)[i][0] < (subs)[j][0]
	})
}

//合并连续的号段
func mergeSub(subs [][2]int64) [][2]int64 {
	//首先排序
	sortSub(subs)
	//如果结束大于等于 后面 的开始，则合并
	for i := 0; i < len(subs)-1; i++ {
		if subs[i][1] >= subs[i+1][0] {
			subs[i][1] = subs[i+1][1]
			subs = DeleteSlice2(subs, i+1)
			i = i - 1
		}
	}
	return subs
}
