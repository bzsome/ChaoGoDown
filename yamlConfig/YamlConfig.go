package yamlConfig

import (
	"gopkg.in/yaml.v3"
	"os"
)

func WriteConfigYaml(fileName string, v interface{}) {
	writeFile, _ := os.Create(fileName)
	encoder := yaml.NewEncoder(writeFile)
	encoder.Encode(v)
	encoder.Close()
	writeFile.Close()
}

func GetConfigYaml(fileName string, v interface{}) {
	readFile, _ := os.Open(fileName) //test.yaml由下一个例子生成
	decode := yaml.NewDecoder(readFile)
	decode.Decode(v)
	readFile.Close()
}
