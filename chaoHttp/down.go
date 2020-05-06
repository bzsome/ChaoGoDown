package chaoHttp

import (
	"chaoDown/yamlConfig"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
)

type Download interface {
	Resolve(request *http.Request) (*http.Response, error)
	Down(request *http.Request) error
}

// 返回文件的相关信息
func Resolve(request *Request) (*Response, error) {
	httpRequest, err := BuildHTTPRequest(request)
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

//初始化下载进度(首先重文件中读取已下载完成的片段)
func initSubs(request *Request) {
	configFile := getConfigFile(request)
	yamlConfig.GetConfigYaml(configFile, request)
	//构造完整的片段
	allSubs, _ := generateSubs(request)

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
		downed := isDowned(one, request.Subeds)
		if !downed {
			tempSubeds = append(tempSubeds, one)
		}
	}
	request.unSubs = tempSubeds
}

func getConfigFile(request *Request) string {
	//读取已下子的片段
	configFile := request.FileName + "." + GetStringMd5(request.URL) + ".yaml"
	return configFile
}

// Down
//支持分段下载，且程序中断重启能够继续下载
func Down(request *Request) error {
	if request.FileName == "" {
		request.FileName = path.Base(request.URL)
	}

	initSubs(request)

	file, err := os.OpenFile(request.FileName, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	defer file.Close()

	waitGroup := &sync.WaitGroup{}
	for i, one := range request.unSubs {
		waitGroup.Add(1)
		done := getDone(request, waitGroup, one, i)
		go downChunk(request, file, one[0], one[1], done)
	}
	waitGroup.Wait()
	return nil
}

//必须要形成闭包，否则one将只取最后一个值
func getDone(request *Request, waitGroup *sync.WaitGroup, one [2]int64, index int) func(err error) {
	done := func(err error) {
		defer waitGroup.Done()
		if err != nil {
			fmt.Printf("down err %10s %10s %s\n", formatFileSize(one[0]), formatFileSize(one[1]), err)
		} else {
			fmt.Printf("down end %10s %10s\n", formatFileSize(one[0]), formatFileSize(one[1]))
			request.Subeds = append(request.Subeds, [2]int64{one[0], one[1]})
			request.unSubs = DeleteSlice(request.Subeds, one)

			configFile := getConfigFile(request)
			yamlConfig.WriteConfigYaml(configFile, request)
		}
	}
	return done
}

//构造完整的片段
func generateSubs(request *Request) ([][2]int64, error) {
	var subs [][2]int64

	response, err := Resolve(request)
	if err != nil {
		return subs, err
	}

	// 支持断点续传
	if response.Range {
		cons := 16
		chunkSize := response.Size / int64(cons)
		for i := 0; i < cons; i++ {
			start := int64(i) * chunkSize
			end := start + chunkSize
			if i == cons-1 {
				end = response.Size
			}
			one := [2]int64{start, end}
			subs = append(subs, one)
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
func downChunk(request *Request, file *os.File, start int64, end int64, chunkDone func(err error)) {
	if chunkDone == nil {
		chunkDone = func(err error) {}
	}

	fmt.Printf("down start %10s %10s\n", formatFileSize(start), formatFileSize(end))

	httpRequest, _ := BuildHTTPRequest(request)
	httpRequest.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	httpClient := BuildHTTPClient()
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		chunkDone(err)
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
				chunkDone(err)
				return
			}
			writeIndex += int64(writeSize)
		}
		if err != nil {
			if err != io.EOF {
				chunkDone(err)
				return
			}
			chunkDone(nil)
			break
		}
	}
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
