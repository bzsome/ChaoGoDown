package chaoHttp

type Request struct {
	Method  string
	URL     string
	Header  map[string]string
	Content []byte
	//已下载的片段,用于重启程序继续下载
	Subeds [][2]int64 `yaml:"Subeds,flow"`
	//未下载的片段
	unSubs   [][2]int64
	fileSize int64
}
