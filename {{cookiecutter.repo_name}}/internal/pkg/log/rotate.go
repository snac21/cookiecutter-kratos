package log

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RotateWriter 自定义的日志轮转写入器
type RotateWriter struct {
	mu sync.Mutex

	// 配置参数
	filename   string
	maxSize    int64 // bytes
	maxAge     int   // days
	maxBackups int
	compress   bool

	// 运行时状态
	file *os.File
	size int64
}

// NewRotateWriter 创建一个新的日志轮转写入器
func NewRotateWriter(filename string, maxSize int, maxAge int, maxBackups int, compress bool) *RotateWriter {
	return &RotateWriter{
		filename:   filename,
		maxSize:    int64(maxSize) * 1024 * 1024, // 转换为字节
		maxAge:     maxAge,
		maxBackups: maxBackups,
		compress:   compress,
	}
}

// Write 实现 io.Writer 接口
func (w *RotateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	writeLen := int64(len(p))
	if writeLen > w.maxSize {
		return 0, fmt.Errorf("write length %d exceeds maximum file size %d", writeLen, w.maxSize)
	}

	if w.file == nil {
		if err = w.openExistingOrNew(len(p)); err != nil {
			return 0, err
		}
	}

	if w.size+writeLen > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = w.file.Write(p)
	w.size += int64(n)

	return n, err
}

// Close 关闭文件
func (w *RotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.close()
}

// close 关闭文件（内部方法，不加锁）
func (w *RotateWriter) close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// openExistingOrNew 打开现有文件或创建新文件
func (w *RotateWriter) openExistingOrNew(writeLen int) error {
	w.mill()

	filename := w.filename
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return w.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %s", err)
	}

	if info.Size()+int64(writeLen) >= w.maxSize {
		return w.rotate()
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return w.openNew()
	}
	w.file = file
	w.size = info.Size()
	return nil
}

// openNew 创建新文件
func (w *RotateWriter) openNew() error {
	err := os.MkdirAll(w.dir(), 0755)
	if err != nil {
		return fmt.Errorf("can't make directories for new logfile: %s", err)
	}

	name := w.filename
	mode := os.FileMode(0644)
	info, err := os.Stat(name)
	if err == nil {
		mode = info.Mode()
		newname := w.backupName(name, time.Now())
		if err := os.Rename(name, newname); err != nil {
			return fmt.Errorf("can't rename log file: %s", err)
		}

		if w.compress {
			go w.compressFile(newname)
		}
	}

	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("can't open new logfile: %s", err)
	}
	w.file = f
	w.size = 0
	return nil
}

// backupName 生成备份文件名
func (w *RotateWriter) backupName(name string, t time.Time) string {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]

	// 格式: service-name.log-yyyy-MM-dd-{index}
	timestamp := t.Format("2006-01-02")

	// 查找当天已有的备份文件数量
	index := 1
	for {
		backupName := fmt.Sprintf("%s-%s-%d%s", prefix, timestamp, index, ext)
		backupPath := filepath.Join(dir, backupName)
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			return backupPath
		}
		index++
	}
}

// rotate 轮转日志文件
func (w *RotateWriter) rotate() error {
	if err := w.close(); err != nil {
		return err
	}
	if err := w.openNew(); err != nil {
		return err
	}
	w.mill()
	return nil
}

// mill 清理旧的日志文件
func (w *RotateWriter) mill() {
	if w.maxBackups == 0 && w.maxAge == 0 {
		return
	}

	files, err := w.oldLogFiles()
	if err != nil {
		return
	}

	var deletes []logInfo

	if w.maxBackups > 0 && w.maxBackups < len(files) {
		deletes = files[w.maxBackups:]
		files = files[:w.maxBackups]
	}
	if w.maxAge > 0 {
		diff := time.Duration(int64(24*time.Hour) * int64(w.maxAge))
		cutoff := time.Now().Add(-1 * diff)

		for _, f := range files {
			if f.timestamp.Before(cutoff) {
				deletes = append(deletes, f)
			}
		}
	}

	for _, f := range deletes {
		os.Remove(filepath.Join(w.dir(), f.Name()))
	}
}

// oldLogFiles 获取旧的日志文件列表
func (w *RotateWriter) oldLogFiles() ([]logInfo, error) {
	files, err := os.ReadDir(w.dir())
	if err != nil {
		return nil, fmt.Errorf("can't read log file directory: %s", err)
	}
	logFiles := []logInfo{}

	prefix, ext := w.prefixAndExt()

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if t, err := w.timeFromName(f.Name(), prefix, ext); err == nil {
			logFiles = append(logFiles, logInfo{t, f})
		}
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

// timeFromName 从文件名中提取时间
func (w *RotateWriter) timeFromName(filename, prefix, ext string) (time.Time, error) {
	if !strings.HasPrefix(filename, prefix) {
		return time.Time{}, fmt.Errorf("mismatched prefix")
	}
	if !strings.HasSuffix(filename, ext) {
		return time.Time{}, fmt.Errorf("mismatched extension")
	}
	ts := filename[len(prefix) : len(filename)-len(ext)]

	// 解析格式: -yyyy-MM-dd-{index}
	parts := strings.Split(ts, "-")
	if len(parts) < 4 {
		return time.Time{}, fmt.Errorf("invalid timestamp format")
	}

	// 重新组合日期部分
	dateStr := strings.Join(parts[1:4], "-")
	return time.Parse("2006-01-02", dateStr)
}

// prefixAndExt 获取文件前缀和扩展名
func (w *RotateWriter) prefixAndExt() (prefix, ext string) {
	filename := filepath.Base(w.filename)
	ext = filepath.Ext(filename)
	prefix = filename[:len(filename)-len(ext)] + "-"
	return prefix, ext
}

// dir 获取日志文件目录
func (w *RotateWriter) dir() string {
	return filepath.Dir(w.filename)
}

// compressFile 压缩文件（如果启用压缩）
func (w *RotateWriter) compressFile(filename string) {
	// 这里可以实现文件压缩逻辑
	// 为了简化，暂时不实现压缩功能
}

// logInfo 日志文件信息
type logInfo struct {
	timestamp time.Time
	os.DirEntry
}

// byFormatTime 按时间排序
type byFormatTime []logInfo

func (b byFormatTime) Less(i, j int) bool {
	return b[i].timestamp.After(b[j].timestamp)
}

func (b byFormatTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFormatTime) Len() int {
	return len(b)
}
