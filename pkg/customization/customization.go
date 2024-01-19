package customization

import (
	"fmt"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
)

func (c *Customizeconfirmation) Check() error {
	c.Result = true
	for _, confirmation := range c.Confirmations {
		err := confirmation.Check()
		if err != nil {
			klog.Error(err.Error())
			c.Result = false
			c.Message = confirmation.GetMessage()
			break
		}
		if !confirmation.GetResult() {
			c.Result = false
			break
		}
	}

	return nil
}

func (c *FileConfirmation) Check() error {
	filePath := c.Path
	if !filepath.IsAbs(filePath) {
		return fmt.Errorf("%s is not a valid path", filePath)
	}

	filePath = util.GetHostPath() + filePath

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.Exist = false
		} else {
			c.Result = false
			c.Message = err.Error()
			return err
		}
	} else {
		c.Exist = true
	}

	c.Result = c.Exist == c.ExpectedExist
	if !c.Exist {
		return nil
	}

	if fileInfo.IsDir() {
		c.Type = "dir"
	} else {
		ok, err := isTextFile(filePath)
		if err != nil {
			c.Result = false
			c.Message = err.Error()
			return err
		}

		if ok {
			c.Type = "text"
		} else {
			c.Type = "binary"
		}
	}

	if c.Type != "text" && len(c.Content) > 0 {
		c.Result = false
		c.Message = fmt.Sprintf("%s is %s, cannot match content \"%s\"", filePath, c.Type, c.Content)
		return fmt.Errorf(c.Message)
	}

	if len(c.Content) > 0 {
		contains, err := containsAllStrings(filePath, c.Content)
		if err != nil {
			c.Result = false
			c.Message = err.Error()
			return err
		}
		if contains {
			c.ContentMatched = true
		} else {
			c.ContentMatched = false
		}

		c.Result = c.ExpectedContentMatched == c.ContentMatched && c.ExpectedExist == c.Exist
	}
	c.Message = ""

	return nil
}

func (c *FileConfirmation) GetResult() bool {
	return c.Result
}

func (c *FileConfirmation) GetMessage() string {
	return c.Message
}
