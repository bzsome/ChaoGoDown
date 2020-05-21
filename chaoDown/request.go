package chaoDown

type Request struct {
	Method  string
	URL     string
	Header  map[string]string
	Content []byte

	//已下载的片段,用于重启程序继续下载，yaml使用数组储值
	//必须公有，否则无法保证至yaml
	Subeds [][2]int64 `yaml:"subeds,flow"`

	unSubs   [][2]int64 //未下载的片段
	fileName string     //从服务器上获得的文件名
	fileSize int64      //请求文件的大小
	Range    bool       //是否支持分段下载
}
