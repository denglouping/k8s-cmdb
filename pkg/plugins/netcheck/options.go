package netcheck

// Options bcs log options
type Options struct {
	Interval        int      `json:"interval" yaml:"interval"`
	Synchronization bool     `json:"synchronization" yaml:"synchronization"`
	CheckDomain     []string `json:"checkDomain" yaml:"checkDomain"`
}

// Validate validate options
func (o *Options) Validate() error {

	return nil
}
