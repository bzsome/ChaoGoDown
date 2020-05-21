package main

import (
	"fmt"

	"github.com/bzsome/ChaoGoDown/chaoDown"
	"github.com/bzsome/ChaoGoDown/utils"
)

func main() {
	url := "https://codeload.github.com/alibaba/flutter-go/zip/master"
	//request配置参数仅第一次有用，第二次将配置文件中读取
	request := &chaoDown.Request{
		Method: "get",
		URL:    url,
		Header: map[string]string{
			"Host":            "pcs.baidu.com",
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.121 Safari/537.36",
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8",
			"Referer":         url,
			"Accept-Encoding": "gzip, deflate, br",
			"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		},
	}
	download := &chaoDown.Downloader{
		PoolSize:  100,
		ChuckSize: 1024 * 100,
		Path:      "downloads",
		Wait:      false,
		//TaskName:  "bzchao",
	}
	for recount := 5; recount > 0; recount-- {
		err := download.Down(request)
		//只有错误状态为重试才继续，否则打印错误，直接退出
		if err == utils.RETRY {
			fmt.Println(err)
			fmt.Println("\n正在重试：", recount-1, "...")
			continue
		} else if err != nil {
			fmt.Println(err)
			return
		} else {
			if !download.Wait {
				download.WaitDone()
			}
			time := download.GetExeTime()
			fmt.Printf("下载用时：%.2f 秒，保存路径：%s", time.Seconds(), download.GetSavePath())
			return
		}
	}

}
