package customization

import (
	"io"
	"os"
	"strings"
)

type CustomizeconfirmationConfig struct {
	Name              string                   `yaml:"name"`
	FileConfirmations []FileConfirmationConfig `yaml:"fileConfirmations"`
}

type Customizeconfirmation struct {
	Name          string         `yaml:"name"`
	Confirmations []Confirmation `yaml:"confirmations"`
	Result        bool           `yaml:"result"`
	Message       string         `yaml:"message"`
}

type Confirmation interface {
	Check() error
	GetResult() bool
	GetMessage() string
}

type FileConfirmationConfig struct {
	Name                   string
	Path                   string
	Content                []string
	ExpectedExist          bool
	ExpectedContentMatched bool
	Type                   string
	Message                string
}

type FileConfirmation struct {
	FileConfirmationConfig
	Result         bool
	Exist          bool
	ContentMatched bool
}

type FileConfirmationResult struct {
	Exist          bool
	ContentMatched bool
	Type           string
}

func isTextFile(path string) (bool, error) {
	// 打开文件
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// 创建一个缓冲区，用于读取文件内容的一部分
	bufferSize := 4096 // 每次读取的块大小
	buffer := make([]byte, bufferSize)

	// 逐块读取文件内容，并判断是否为文本文件
	for {
		// 从文件中读取一部分内容到缓冲区
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return false, err
		}

		// 检查读取的内容是否包含非 ASCII 字符
		for _, b := range buffer[:n] {
			if b < 32 && b != 9 && b != 10 && b != 13 {
				return false, nil
			}
		}

		// 如果已经读取到文件末尾，则退出循环
		if err == io.EOF {
			break
		}
	}

	return true, nil
}

func containsAllStrings(path string, targets []string) (bool, error) {
	// 打开文件
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// 创建一个缓冲区，用于读取文件内容的一部分
	bufferSize := 4096 // 每次读取的块大小
	buffer := make([]byte, bufferSize)

	// 创建一个 map 用于记录目标字符串的匹配情况
	matchedList := make(map[string]bool)
	for _, target := range targets {
		matchedList[target] = false
	}

	// 逐块读取文件内容，并在每个块中查找目标字符串
	for {
		// 从文件中读取一部分内容到缓冲区
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return false, err
		}

		// 将读取的内容转换为字符串
		content := string(buffer[:n])

		// 在读取的内容中查找目标字符串
		for _, target := range targets {
			if strings.Contains(content, target) {
				matchedList[target] = true
			}
		}

		// 如果已经读取到文件末尾，则退出循环
		if err == io.EOF {
			break
		}
	}

	// 检查目标字符串的匹配情况
	for _, matched := range matchedList {
		if !matched {
			return false, nil
		}
	}

	return true, nil
}

func containsString(path string, targets []string) (bool, error) {
	// 打开文件
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// 创建一个缓冲区，用于读取文件内容的一部分
	bufferSize := 4096 // 每次读取的块大小
	buffer := make([]byte, bufferSize)

	// 逐块读取文件内容，并在每个块中查找目标字符串
	for {
		// 从文件中读取一部分内容到缓冲区
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return false, err
		}

		// 将读取的内容转换为字符串
		content := string(buffer[:n])

		// 在读取的内容中查找目标字符串
		for _, target := range targets {
			if strings.Contains(content, target) {
				return true, nil
			}
		}

		// 如果已经读取到文件末尾，则退出循环
		if err == io.EOF {
			break
		}
	}

	return true, nil
}
