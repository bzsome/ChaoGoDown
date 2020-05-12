package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"ChaoGoDown/chaoHttp"
)

//重试次数
var recount = 5

// flag包实现了命令行参数的解析。
func main() {
	/*
	   定义变量接收控制台参数
	*/
	// 用户
	var url string
	// 密码
	var PoolSize int
	// 主机名
	var ChuckSize int64
	// 用户
	var path string

	// StringVar用指定的名称、控制台参数项目、默认值、使用信息注册一个string类型flag，并将flag的值保存到p指向的变量
	flag.StringVar(&url, "url", "", "url地址, 必填")
	flag.IntVar(&PoolSize, "n", 10, "并行数量,默认为10")
	flag.Int64Var(&ChuckSize, "c", 1024*100, "数据块大小，默认100K")
	flag.StringVar(&path, "path", "downloads", "保存路径")

	// 从arguments中解析注册的flag。必须在所有flag都注册好而未访问其值时执行。未注册却使用flag -help时，会返回ErrHelp。
	flag.Parse()

	fmt.Println("参数：", os.Args)

	request := &chaoHttp.Request{
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
	download := &chaoHttp.Downloader{
		PoolSize:  PoolSize,
		ChuckSize: ChuckSize,
		Path:      path,
	}
	if !strings.HasPrefix(url, "http") {
		fmt.Println("url不能为空，", url)
		return
	}

	for ; recount > 0; recount-- {
		err := download.Init(request)
		if err != nil {
			fmt.Println(err)
			fmt.Println("\n正在重试：", recount-1, "...")
		} else {
			break
		}
	}

	err := download.Down()
	if err != nil {
		fmt.Println(err)
	}
	time := download.GetExeTime()
	fmt.Printf("下载用时：%.2f 秒", time.Seconds())

}
