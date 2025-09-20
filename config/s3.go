package config

type S3Config struct {
	Endpoint  string `yaml:"endpoint"`
	AccessKey string `yaml:"access-key"`
	SecretKey string `yaml:"secret-key"`
	SSL       bool   `yaml:"ssl"`
	Region    string `yaml:"region"`
}
