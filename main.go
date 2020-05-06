package main

import (
	"chaoDown/chaoHttp"
	"fmt"
)

func main() {
	request := &chaoHttp.Request{
		Method: "get",
		URL:    "https://down.qq.com/qqweb/PCQQ/PCQQ_EXE/PCQQ2020.exe",
		Header: map[string]string{
			"Host":            "github.com",
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.121 Safari/537.36",
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8",
			"Referer":         "http://www.bzchao.com",
			"Accept-Encoding": "gzip, deflate, br",
			"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		},
	}
	got, err := chaoHttp.Resolve(request)
	if err != nil {
		fmt.Println(err)
	}
	err = chaoHttp.Down(request)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(got)
}
