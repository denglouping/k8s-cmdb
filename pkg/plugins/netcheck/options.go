package netcheck

// Options bcs log options
type Options struct {
	Interval      int    `json:"interval" yaml:"interval"`
	LabelSelector string `json:"labelSelector" yaml:"labelSelector"`
}

// Validate validate options
func (o *Options) Validate() error {

	return nil
}
