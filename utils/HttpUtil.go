package utils

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
)

//分段下载，指定下载的起始
func DownChunk(httpRequest *http.Request, osFile *os.File, start int64, end int64, done func(err error)) {
	if done == nil {
		done = func(err error) {}
	}

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
			writeSize, err := osFile.WriteAt(buf[0:n], writeIndex)

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

func BuildHTTPRequest(method string, url string, header map[string]string) (*http.Request, error) {
	// Build request
	httpRequest, err := http.NewRequest(strings.ToUpper(method), url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		httpRequest.Header.Add(k, v)
	}
	return httpRequest, nil
}

func BuildHTTPClient() *http.Client {
	// Cookie handle
	jar, _ := cookiejar.New(nil)

	return &http.Client{Jar: jar}
}
