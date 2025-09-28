package config

type FTPConfig struct {
	Host     string `yaml:"host"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Port     int    `yaml:"port"` // Optional, default 21 for FTP, 22 for SFTP
}
