package convert

import (
	"github.com/jinzhu/copier"
)

// Copy 静态方法，拷贝src到dst，内存地址需传入指针
func Copy(source, dst any) error {
	return copier.Copy(dst, source)
}
