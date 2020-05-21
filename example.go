package main

import (
	"fmt"

	"github.com/bzsome/ChaoGoDown/chaoDown"
)

func main() {
	request := &chaoDown.Request{
		Method: "get",
		URL:    "https://github.com/alibaba/nacos/releases/download/1.2.1/nacos-server-1.2.1.tar.gz",
		Header: map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.121 Safari/537.36",
		},
	}
	download := &chaoDown.Downloader{
		//PoolSize:  100,
		//ChuckSize: 1024 * 100,
		Path: "downloads",
	}

	err := download.Down(request)
	if err != nil {
		fmt.Println(err)
	} else {
		time := download.GetExeTime()
		fmt.Printf("下载用时：%.2f 秒，保存路径：%s", time.Seconds(), download.GetSavePath())
	}

}
