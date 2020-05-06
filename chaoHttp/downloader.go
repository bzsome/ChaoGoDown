package chaoHttp

import (
	"chaoDown/yamlConfig"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/shopspring/decimal"
	"github.com/xxjwxc/gowp/workpool"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
)

type Downloader struct {
	request  *Request
	response *Response
	file     *os.File
	FileName string
	//线程池大小
	PoolSize int
	//每个线程池下载块大小
	ChuckSize int64
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

	fmt.Print("->文件信息")
	down.request = request
	if down.FileName == "" {
		down.FileName = path.Base(request.URL)
	}

	file, err := os.OpenFile(down.FileName, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	down.file = file
	down.request = request
	down.response, err = down.Resolve()
	if err != nil {
		return err
	}
	fmt.Print("->数据块信息")
	down.request.fileSize = down.response.Size
	if down.ChuckSize <= 0 {
		return errors.New("subSize大小不能为0")
	}
	down.initSubs()

	fmt.Println("->初始化完成")
	return nil
}

//初始化下载进度(首先重文件中读取已下载完成的片段)
func (down *Downloader) initSubs() {
	configFile := getConfigFile(down.request)
	yamlConfig.GetConfigYaml(configFile, down.request)
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
}

//获得配置文件名
func getConfigFile(request *Request) string {
	configFile := path.Base(request.URL) + "." + GetStringMd5(request.URL) + ".yaml"
	return configFile
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
	//使用单独的线程输出进度
	/*	go func() {
		for {
			time.Sleep(time.Second)
			printRate(down.request)
		}
	}()*/
	wp.Wait()
	fmt.Println("OK，下载完成！")
	return nil
}

func (down *Downloader) doOneChuck(one [2]int64) func() error {
	return func() error {
		done := downDone(down.request, one)
		downChunk(down.request, down.file, one[0], one[1], done)
		return nil
	}
}

//下载完成回调
func downDone(request *Request, one [2]int64) func(err error) {
	done := func(err error) {
		if err != nil {
			fmt.Printf("down err %10s %10s %s\n", humanize.Bytes(uint64(one[0])), humanize.Bytes(uint64(one[1])), err)
		} else {
			printRate(request)
			request.Subeds = append(request.Subeds, [2]int64{one[0], one[1]})
			request.unSubs = DeleteSlice(request.Subeds, one)

			configFile := getConfigFile(request)
			yamlConfig.WriteConfigYaml(configFile, request)
		}
	}
	return done
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
		//不支持断点续传，则一次性全部下载
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
func downChunk(request *Request, file *os.File, start int64, end int64, done func(err error)) {
	if done == nil {
		done = func(err error) {}
	}

	fmt.Printf("chunk   start - end   %10s -%10s\n", formatFileSize(start), formatFileSize(end))

	httpRequest, _ := BuildHTTPRequest(request)
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
			writeSize, err := file.WriteAt(buf[0:n], writeIndex)
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

func DeleteSlice(list [][2]int64, one [2]int64) [][2]int64 {
	ret := make([][2]int64, 0, len(list))
	for _, val := range list {
		if val[0] == one[0] && val[1] == one[1] {
			ret = append(ret, val)
		}
	}
	return ret
}

func getDownTotal(request *Request) int64 {
	var total = int64(0)
	for _, one := range request.Subeds {
		size := one[1] - one[0]
		total = total + size
	}
	return total
}
