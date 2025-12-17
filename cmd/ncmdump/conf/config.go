package conf

type Config struct {
	Path struct {
		Source []string `yaml:"source"`
		Target string   `yaml:"target"`
	}
}

func (c *Config) GetSourcePath() []string {
	if c == nil {
		return nil
	}
	return c.Path.Source
}

func (c *Config) GetTargetPath() string {
	if c == nil {
		return ""
	}
	return c.Path.Target
}
