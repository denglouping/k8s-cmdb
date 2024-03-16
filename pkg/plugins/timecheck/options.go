package timecheck

// Options bcs log options
type Options struct {
	Interval    int    `json:"interval" yaml:"interval"`
	TimeServers string `json:"timeServers" yaml:"timeServers"`
}

// Validate validate options
func (o *Options) Validate() error {
	return nil
}
