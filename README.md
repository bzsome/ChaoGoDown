## ChaoGoDown
基于Go语言的多线程文件下载

新增功能：通过配置文件的方式保存下载进度，程序中断重启能够继续下载
新增功能：已打包成exe，可直接下载exe，在cmd环境下执行

## 主要功能
- [√] 多线程分段下载
- [√] 自定义请求头
- [√] 下载进度保存
- [√] 指定线程数量
- [√] 指定下载块大小
- [√] 接收cmd命令
    
## 使用说明
    1、如有片段提示下载失败，重新执行程序即可，直到没有任何错误提示
    2、如需完全重新下载文件，请删除yaml配置文件。继续下载不用删除！
    
## 使用技巧
    1、下载github文件。调小chunkSize，调大poolSize
    2、github总是连接失败，由于github好像是随机服务器，有些服务器国内屏蔽了，重新执行程序即可
    3、后面的片段下载很慢，正常的，毕竟线程数量变少了。可以尝试，减小chunkSize后下载剩余的片段
    4、如果chunkSize设置过大，下载此片段的时间过长(或者中途响应超时)，导致片段下载失败
    5、出现net/http: TLS handshake timeout，建议逐渐调小chunkSize。

## CMD命令
    示例(从github下载nacos)：
    downCmd.exe -n 10 -c 10240 -url https://github.com/alibaba/nacos/releases/download/1.2.1/nacos-server-1.2.1.tar.gz
    
 相关参数
 
      -c int
            数据块大小，默认100K (default 102400)
      -n int
            并行数量,默认为10 (default 10)
      -path string
            保存路径 (default "downloads")
      -url string
            url地址, 必填
            
## 建议参数

- PoolSize 线程池大小(最终速度：ChuckSize*poolSize = 1024K*1024M*poolSize)
- ChuckSize 每个线程池下载块大小（github建议1024 * 4，国内建议1024*1024） 

## Example App
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


## 项目结构
```
ChaoGoDown
│   README.md           说明文件
│   go.mod              模块管理
│   main.go             主模块，功能齐全，主要用户调试
│   downCmd.go          接收CMD指令，可以打包成可执行文件
│   example.go          简单的示例代码
│
└───chaoDown        主要的包
│   │   downloader.go       下载器。用下载器来下载url，支持多线程、分段下载
│   │   request.go          请求配置，包含请求url的信息
│   
└───utils           工具包
    │   YamlConfig.go       Yaml配置，主要用于保存下载进度，中断继续下载
    │   Clone.go            对象复制，主要用于默认参数的读取
```
