package config

type FTPConfig struct {
	Host     string `yaml:"host"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Port     int    `yaml:"port"` // Optional, Standard 21 für FTP, 22 für SFTP
}
