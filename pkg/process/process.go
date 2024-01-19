package process

import (
	"bufio"
	"context"
	"fmt"
	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/moby/sys/mountinfo"
	"github.com/shirou/gopsutil/process"
	"k8s-cmdb/pkg/util"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// HOST_PROC

var (
	mnt      NS = "mnt"
	hostPath    = util.GetHostPath()
)

func GetProcessNS(pid int32, ns NS) (syscall.Stat_t, error) {
	mntNSFile := fmt.Sprintf("/proc/%d/ns/%s", pid, ns)

	var stat syscall.Stat_t
	err := syscall.Stat(mntNSFile, &stat)
	return stat, err
}

func GetProcessServiceConfigfiles(starter string) (map[string]string, error) {
	serviceFiles := make(map[string]string)
	var err error
	serviceFiles[starter], err = GetConfigfile(fmt.Sprintf("%s/usr/lib/systemd/system/", hostPath) + starter)
	if err != nil {
		return nil, err
	}

	fileInfo, err := os.Stat(fmt.Sprintf("%s/usr/lib/systemd/system/%s.d", hostPath, starter))
	if err == nil {
		if fileInfo.IsDir() {
			files, err := os.ReadDir(fmt.Sprintf("%s/usr/lib/systemd/system/%s.d", hostPath, starter))
			if err != nil {
				return nil, err
			}

			for _, serviceFile := range files {
				if !serviceFile.IsDir() {
					serviceFiles[serviceFile.Name()], err = GetConfigfile(fmt.Sprintf("%s/usr/lib/systemd/system/%s.d/%s", hostPath, starter, serviceFile.Name()))
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return serviceFiles, err
}

// other, systemd, container, crontab, cmdline
func GetStarter(pid, ppid int32) (string, error) {
	starter := "other"
	switch ppid {
	case 1:
		cgroupFile := fmt.Sprintf("%s/proc/%d/cgroup", hostPath, pid)
		file, err := os.Open(cgroupFile)
		if err != nil {
			klog.Errorf("Get process cgroup info failed: %s", err.Error())
			return starter, err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "/system.slice/") {
				starter = filepath.Base(strings.Split(line, ":")[2])
				break
			}
		}

	default:
		parent, err := process.NewProcess(ppid)
		if err != nil {
			klog.Errorf("Get process parent info failed: %s", err.Error())
			return starter, err
		}

		parentName, err := parent.Name()
		if err != nil {
			klog.Errorf("Get process parentName failed: %s", err.Error())
			return starter, err
		}

		if parentName == "cron" {
			starter = "crontab"
		} else if strings.Contains(parentName, "runc") || strings.Contains(parentName, "containerd-shim") {
			starter = "container"
		} else if strings.Contains(parentName, "-bash") || strings.Contains(parentName, "-sh") {
			starter = "cmdline"
		}

	}

	return starter, nil
}

func GetConfigfile(path string) (string, error) {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(contentBytes), nil
}

func GetSystemdConfig(pid int32) (err error) {
	conn, err := dbus.NewWithContext(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating a dbus connection:", err)
		os.Exit(1)
	}
	defer conn.Close()

	name, err := conn.GetUnitNameByPID(context.Background(), uint32(pid))
	if err != nil {
		return err
	}

	units, err := conn.ListUnitsContext(context.Background())
	if err != nil {
		return err
	}

	for _, unit := range units {
		if unit.Name == name {

		}
	}
	return nil
}

func GetMountInfoSourcePath(source string) (string, error) {
	f, err := os.Open(fmt.Sprintf("%s/proc/1/mountinfo", util.GetHostPath()))
	if err != nil {
		return "", fmt.Errorf("open path %s failed: %s", fmt.Sprintf("%s/proc/1/mountinfo", util.GetHostPath()), err.Error())
	}

	mountInfoList, err := mountinfo.GetMountsFromReader(f, nil)
	if err != nil {
		return "", fmt.Errorf("Get process mountinfo failed: %s", err.Error())
	}

	for _, mountInfo := range mountInfoList {
		if mountInfo.Source == source {
			return mountInfo.Mountpoint, nil
		}
	}
	return "", fmt.Errorf("Get process mountinfo failed: notfound")

}

func GetConfigfileList(params []string, pid int32) (map[string]string, error) {
	configFiles := make(map[string]string)

	// 获取该进程mount信息
	f, err := os.Open(fmt.Sprintf("%s/proc/%d/mountinfo", util.GetHostPath(), pid))
	if err != nil {
		klog.Errorf("Get process %d ConfigFiles failed: %s", pid, err.Error())
	}

	mountInfoList, err := mountinfo.GetMountsFromReader(f, nil)
	if err != nil {
		klog.Errorf("Get process %d ConfigFiles failed: %s", pid, err.Error())
	}
	for _, param := range params {
		re := regexp.MustCompile(`(?:^|=)(/[a-zA-Z0-9\._-]+)+`)
		//re := regexp.MustCompile(`(/[^/]+)+`)
		paths := re.FindAllString(param, -1)

		for _, configFilepath := range paths {
			configFilepath = strings.TrimPrefix(configFilepath, "=")
			if strings.HasSuffix(configFilepath, "sock") {
				continue
			}

			for _, mountInfo := range mountInfoList {
				// 还需要判断 /install-cni.sh 此类情况  // 获取到work目录，并找到对应的merged目录
				if mountInfo.Mountpoint == "/" || mountInfo.Source == "tmpfs" {
					continue
				}
				if strings.Contains(configFilepath, mountInfo.Mountpoint) {
					sourcePath, err := GetMountInfoSourcePath(mountInfo.Source)
					if err != nil {
						klog.Errorf("Get process %d ConfigFiles failed: %s", pid, err.Error())
					}

					remainingPath := strings.Replace(configFilepath, mountInfo.Mountpoint, "", -1)
					if remainingPath != "" {
						configFilepath = fmt.Sprintf("%s/%s/%s", sourcePath, mountInfo.Root, remainingPath)
					} else {
						configFilepath = fmt.Sprintf("%s/%s", sourcePath, mountInfo.Root)
					}

					break
				}
			}

			processConfigFilepath := fmt.Sprintf("%s/%s", hostPath, configFilepath)
			fileInfo, err := os.Stat(processConfigFilepath)
			if err != nil {
				klog.Infof("%d Get GetConfigfile %s content failed: %s", pid, configFilepath, err.Error())
				configFiles[configFilepath] = err.Error()
				continue
			}

			mode := fileInfo.Mode()

			if mode.IsDir() {
				configFiles[configFilepath] = "dir"
			} else if mode.IsRegular() {
				if mode&0111 != 0 {
					configFiles[configFilepath] = "executable"
				} else if fileInfo.Size() > 1024*1024*5 {
					configFiles[configFilepath] = "data"
				} else {
					contentBytes, err := os.ReadFile(processConfigFilepath)
					if err != nil {
						return nil, err
					}

					configFiles[configFilepath] = string(contentBytes)
				}
			} else {
				configFiles[configFilepath] = "other"
			}

		}

	}

	return configFiles, nil

}

func GetProcessInfo(exe string, id int32) (*ProcessInfo, error) {
	var processList []*process.Process
	var err error

	result, ok := util.GetSyncCache("processList")
	if !ok {
		processList, err = process.Processes()
		if err != nil {
			klog.Errorf("Get processList failed: %s", err.Error())
			return nil, err
		}
		util.DefaultCache.Set("processList", processList, time.Minute)
	} else {
		processList = result.([]*process.Process)
	}

	if exe == "" && id == 0 {
		return nil, fmt.Errorf("exe is %s, id is %d, not valid", exe, id)
	}

	processInfo := &ProcessInfo{}

	for _, p := range processList {
		if id != 0 && p.Pid != id {
			continue
		}

		processInfo.Params, err = p.CmdlineSlice()
		if err != nil {
			klog.Errorf("Get process cmdline info failed: %s", err.Error())
			return nil, err
		}

		if len(processInfo.Params) == 0 {
			continue
		}
		processInfo.BinaryPath = processInfo.Params[0]

		filename := filepath.Base(processInfo.BinaryPath)
		if filename != exe && exe != "" {
			continue
		}

		cmdline, err := p.Cmdline()
		if err != nil {
			klog.Errorf("Get process cmdline info failed: %s", err.Error())
			return nil, err
		}
		if cmdline == "" {
			continue
		}

		//processInfo.Env, err = p.Environ()
		//if err != nil {
		//	klog.Errorf("Get process env info failed: %s", err.Error())
		//	//return nil, err
		//}

		ppid, err := p.Ppid()
		if err != nil {
			klog.Errorf("Get process ppid info failed: %s", err.Error())
			return nil, err
		}

		processInfo.Starter, err = GetStarter(p.Pid, ppid)
		if err != nil {
			klog.Errorf("Get process starter info failed: %s", err.Error())
			return nil, err
		}

		// 有些进程会有默认读取的配置文件路径
		if strings.Contains(exe, "docker") {
			configFileFlag := false
			for _, param := range processInfo.Params {
				if strings.Contains(param, "config-file") {
					configFileFlag = true
					break
				}
			}
			if !configFileFlag {
				processInfo.Params = append(processInfo.Params, "--config-file")
				processInfo.Params = append(processInfo.Params, "/etc/docker/daemon.json")
			}
		} else if strings.Contains(exe, "containerd") {
			configFileFlag := false
			for _, param := range processInfo.Params {
				if strings.Contains(param, "--config") {
					configFileFlag = true
					break
				}
			}

			if !configFileFlag {
				processInfo.Params = append(processInfo.Params, "--config")
				processInfo.Params = append(processInfo.Params, "/etc/containerd/config.toml")
			}
		} else if strings.Contains(exe, "coredns") {
			processInfo.Params = append(processInfo.Params, "/etc/resolv.conf")
		}

		if len(processInfo.Params) > 1 {
			processInfo.ConfigFiles, err = GetConfigfileList(processInfo.Params[1:], p.Pid)
			if err != nil {
				klog.Errorf("Get process %d ConfigFiles failed: %s", p.Pid, err.Error())
			}
		}

		if strings.HasSuffix(processInfo.Starter, ".service") {
			processInfo.ServiceFiles, err = GetProcessServiceConfigfiles(processInfo.Starter)
			if err != nil {
				klog.Infof("Get process %d ServiceFiles failed: %s", p.Pid, err.Error())
			}
		}

		processInfo.Status, err = p.Status()
		if err != nil {
			klog.Infof("Get process %d status failed: %s", p.Pid, err.Error())
		}

		return processInfo, nil
	}

	return nil, fmt.Errorf("%s process not found", exe)
}
