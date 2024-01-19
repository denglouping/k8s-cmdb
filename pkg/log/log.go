package log

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

type LogFile struct {
	filename      string
	file          *os.File
	pos           int64
	ino           uint64
	LogChann      chan string
	searchKeyList []string
}

func openLogFile(filename string) (*LogFile, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		file.Close()
		return nil, fmt.Errorf("failed to get inode number for %s", filename)
	}

	return &LogFile{filename: filename, file: file, pos: info.Size(), ino: stat.Ino, LogChann: make(chan string)}, nil
}

func (f *LogFile) SetSearchKey(searchKeyList []string) {
	f.searchKeyList = searchKeyList
}

func (f *LogFile) Start() {
	go func() {
		for {
			err := f.checkNewEntries()
			if err != nil {
				fmt.Println("Error checking file:", err)
				close(f.LogChann)
				break
			}

			time.Sleep(10 * time.Second)
		}
	}()
}

func (f *LogFile) checkNewEntries() error {
	info, err := f.file.Stat()
	if err != nil {
		return err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get inode number for %s", f.filename)
	}

	if stat.Ino != f.ino {
		// 文件已经被轮转，关闭旧的文件并打开新的文件
		f.file.Close()

		newFile, err := openLogFile(f.filename)
		if err != nil {
			return err
		}

		*f = *newFile
	}

	if info.Size() > f.pos {
		f.file.Seek(f.pos, io.SeekStart)
		scanner := bufio.NewScanner(f.file)
		for scanner.Scan() {
			line := scanner.Text()

			// mce: [Hardware Error]
			if f.searchKeyList != nil && len(f.searchKeyList) > 0 {
				for _, key := range f.searchKeyList {
					if strings.Contains(line, key) {
						f.LogChann <- line
					}
				}
			} else {
				f.LogChann <- line
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}

		f.pos = info.Size()
	}

	return nil
}

func NewLogFile(path string) *LogFile {
	logFile, err := openLogFile(path)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil
	}

	return logFile
}
