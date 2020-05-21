package chaoDown

import (
	"errors"
	"fmt"
	"mime"
	"os"
	"path"
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
	Wait      bool   //是否等待结束后才返回,false直接返回

	request      *Request
	osFile       *os.File // 保存至本地文件的file对象
	fileFullName string   // 文件夹+文件名
	configFile   string   // 配置文件名

	chunkIndex int64              //下载块序号
	chunkMutex sync.Mutex         //线程锁
	statTime   time.Time          //下载开始时间
	endTime    time.Time          //下载结束时间
	wp         *workpool.WorkPool //用户下载的线程池
}

// 返回文件的相关信息
func (down *Downloader) Resolve() error {
	httpRequest, err := utils.BuildHTTPRequest(down.request.Method, down.request.URL, down.request.Header)
	if err != nil {
		return err
	}

	// Use "Range" header to resolve  请求长度为0的判断，以便获得文件信息
	httpRequest.Header.Add("Range", "bytes=0-0")
	httpClient := utils.BuildHTTPClient()
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
			total := utils.SubLastSlash(contentRange)
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

// Down
//支持分段下载，且程序中断重启能够继续下载
func (down *Downloader) Down(request *Request) error {
	//初始化下载文件信息
	err := down.init(request)
	if err != nil {
		return err
	}

	fmt.Println("->开始下载")
	//创建线程池下载文件
	down.statTime = time.Now()

	down.wp = workpool.New(down.PoolSize)

	for _, oneChuck := range down.request.unSubs {
		//注意闭包(由于外部有for循环，因此这里单独一个方法来返回)
		doOneChuck := down.doOneChuck(oneChuck)
		down.wp.Do(doOneChuck)
	}

	if down.Wait {
		down.WaitDone()
	}
	return nil
}

func (down *Downloader) WaitDone() {
	down.wp.Wait()
	defer down.osFile.Close()
	down.endTime = time.Now()
	if len(down.request.Subeds) == 1 {
		fmt.Println("OK，下载完成！")
	} else {
		fmt.Println("ERR，部分片段失败，请重试！")
	}
}

func (down *Downloader) doOneChuck(one [2]int64) func() error {
	return func() error {
		down.chunkMutex.Lock()
		down.chunkIndex = down.chunkIndex + 1
		fmt.Printf("chunk[%3d]   start - end   %10s -%10s\n", down.chunkIndex, humanize.Bytes(uint64(one[0])), humanize.Bytes(uint64(one[1])))
		down.chunkMutex.Unlock()

		httpRequest, err := utils.BuildHTTPRequest(down.request.Method, down.request.URL, down.request.Header)
		if err != nil {
			return err
		}

		done := down.downDone(one)
		utils.DownChunk(httpRequest, down.osFile, one[0], one[1], done)
		return nil
	}
}

//下载完成回调
func (down *Downloader) downDone(one [2]int64) func(err error) {
	done := func(err error) {
		if err != nil {
			fmt.Printf("down err %10s %10s %s\n", humanize.Bytes(uint64(one[0])), humanize.Bytes(uint64(one[1])), err)
		} else {
			/*并发时涉及文件操作，注意线程安全*/
			down.chunkMutex.Lock()
			defer down.chunkMutex.Unlock()

			//1，先计算下载进度
			request := down.request
			request.Subeds = append(request.Subeds, one)
			request.unSubs = utils.DeleteSliceObject(request.Subeds, one)
			request.Subeds = utils.MergeSub(request.Subeds)
			utils.WriteConfigYaml(down.configFile, request)

			//2.再打印下载进度(否则不精准)
			down.printRate()
		}
	}
	return done
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

//打印下载进度
func (down *Downloader) printRate() {
	fileSize := down.request.fileSize
	total := utils.GetDownTotal(down.request.Subeds)

	decimal.DivisionPrecision = 2
	ds := decimal.NewFromInt(total * 100)
	fmt.Printf("\r%s", strings.Repeat(" ", 35))
	rate := ds.Div(decimal.NewFromFloat(float64(fileSize)))

	fmt.Printf("\rDownloading...%s %% \t (%s/%s) complete\n", rate, humanize.Bytes(uint64(total)), humanize.Bytes(uint64(fileSize)))
}

//初始化下载进度(首先重文件中读取已下载完成的片段)
func (down *Downloader) initSubs() error {
	if down.request.fileSize <= 0 {
		return errors.New("无法获得文件大小")
	}

	down.configFile = down.fileFullName + ".yaml"
	utils.GetConfigYaml(down.configFile, down.request)
	down.request.Subeds = utils.MergeSub(down.request.Subeds)

	//构造完整的片段
	allSubs, _ := down.generateSubs()

	for _, one := range allSubs {
		//判断此段是否已下载，未下载则加入未下载集合
		downed := utils.HasSubset(one, down.request.Subeds)
		if !downed {
			down.request.unSubs = append(down.request.unSubs, one)
		}
	}
	return nil
}

//构造完整下载的片段
func (down *Downloader) GetExeTime() time.Duration {
	return down.endTime.Sub(down.statTime)
}

func (down *Downloader) GetSavePath() string {
	return down.fileFullName
}
