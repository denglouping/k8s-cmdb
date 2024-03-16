package process

type ProcessInfo struct {
	Starter      string // the way start this process
	BinaryPath   string //
	Params       []string
	Env          []string
	ConfigFiles  map[string]string
	ServiceFiles map[string]string
	Status       string
	// 配置文件修改时间，进程启动时间，
}

type ProcessStatus struct {
	Name       string // the way start this process
	Pid        int32
	Status     string
	CreateTime int64
	CpuTime    float64

	// 配置文件修改时间，进程启动时间，
}

type NS string
