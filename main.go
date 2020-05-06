package main

import (
	"chaoDown/chaoHttp"
	"fmt"
)

//重试次数
var recount = 5

func main() {
	url := "https://github.com/iikira/BaiduPCS-Go/releases/download/v3.6.2/BaiduPCS-Go-v3.6.2-windows-x64.zip"
	fmt.Println(url)
	//url2 := "https://down.qq.com/qqweb/PCQQ/PCQQ_EXE/PCQQ2020.exe"
	request := &chaoHttp.Request{
		Method: "get",
		URL:    url,
		Header: map[string]string{
			"Host":            "github.com",
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.121 Safari/537.36",
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8",
			"Referer":         "https://github.com/iikira/BaiduPCS-Go/releases/download/v3.6.2/BaiduPCS-Go-v3.6.2-windows-x64.zip",
			"Accept-Encoding": "gzip, deflate, br",
			"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		},
	}
	download := &chaoHttp.Downloader{
		PoolSize:  100,
		ChuckSize: 1024,
	}
	for ; recount > 0; recount-- {
		err := download.Init(request)
		if err != nil {
			fmt.Println(err)
			fmt.Println("真正重试：", recount-1, "...")
		} else {
			break
		}

	}

	err := download.Down()
	if err != nil {
		fmt.Println(err)
	}
}
