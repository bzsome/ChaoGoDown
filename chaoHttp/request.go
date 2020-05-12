package chaoHttp

type Request struct {
	Method  string
	URL     string
	Header  map[string]string
	Content []byte

	//已下载的片段,用于重启程序继续下载，yaml使用数组储值
	//必须公有，否则无法保证至yaml
	Subeds [][2]int64 `yaml:"subeds,flow"`

	//未下载的片段
	unSubs [][2]int64
	//请求文件的大小
	fileSize int64
}
