package utils

import (
	"sort"
	"strings"
)

func SubLastSlash(str string) string {
	index := strings.LastIndex(str, "/")
	if index != -1 {
		return str[index+1:]
	}
	return ""
}

//根据下标删除
func DeleteSliceIndex(list [][2]int64, index int) [][2]int64 {
	return append(list[:index], list[index+1:]...)
}

//删除指定的对象
func DeleteSliceObject(list [][2]int64, one [2]int64) [][2]int64 {
	ret := make([][2]int64, 0, len(list))
	for _, val := range list {
		if val[0] == one[0] && val[1] == one[1] {
			ret = append(ret, val)
		}
	}
	return ret
}

//合并连续的号段
func MergeSub(subs [][2]int64) [][2]int64 {
	//首先排序
	SortSub(subs)
	//如果结束大于等于 后面 的开始，则合并
	for i := 0; i < len(subs)-1; i++ {
		if subs[i][1] >= subs[i+1][0] {
			subs[i][1] = subs[i+1][1]
			subs = DeleteSliceIndex(subs, i+1)
			i = i - 1
		}
	}
	return subs
}

//判断是否是子集
func HasSubset(one [2]int64, Subeds [][2]int64) bool {
	//已下载的必须完全包含此段
	for _, sed := range Subeds {
		if sed[0] <= one[0] && sed[1] >= one[1] {
			return true
		}
	}
	return false
}

//对号段排序
func SortSub(subs [][2]int64) {
	sort.Slice(subs, func(i, j int) bool {
		return (subs)[i][0] < (subs)[j][0]
	})
}

//获得已下载号段的总长度
func GetDownTotal(subeds [][2]int64) int64 {
	var total = int64(0)
	for _, one := range subeds {
		size := one[1] - one[0]
		total = total + size
	}
	return total
}
