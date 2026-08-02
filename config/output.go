package config

import (
	"net/url"
	"strconv"
	"strings"
)

type OutputTarget struct {
	Path string `yaml:"path" json:"path"`
	Type string `yaml:"type" json:"type"`

	// S3-specific configuration
	Endpoint  string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	AccessKey string `yaml:"access-key,omitempty" json:"access-key,omitempty"`
	SecretKey string `yaml:"secret-key,omitempty" json:"secret-key,omitempty"`
	SSL       *bool  `yaml:"ssl,omitempty" json:"ssl,omitempty"`
	Region    string `yaml:"region,omitempty" json:"region,omitempty"`

	// FTP/SFTP-specific configuration
	Host     string `yaml:"host,omitempty" json:"host,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	TLS      bool   `yaml:"tls,omitempty" json:"tls,omitempty"` // FTP: explicit FTPS (AUTH TLS)

	// SFTP host key verification
	KnownHosts                string `yaml:"known-hosts,omitempty" json:"known-hosts,omitempty"`
	InsecureSkipHostKeyVerify bool   `yaml:"insecure-skip-host-key-verification,omitempty" json:"insecure-skip-host-key-verification,omitempty"`
}

// GetS3Config extracts the S3 configuration from the OutputTarget
func (ot *OutputTarget) GetS3Config() S3Config {
	ssl := true // default value
	if ot.SSL != nil {
		ssl = *ot.SSL
	}
	return S3Config{
		Endpoint:  ot.Endpoint,
		AccessKey: ot.AccessKey,
		SecretKey: ot.SecretKey,
		SSL:       ssl,
		Region:    ot.Region,
	}
}

// GetFTPConfig extracts the FTP configuration from the OutputTarget
func (ot *OutputTarget) GetFTPConfig() FTPConfig {
	host := ot.Host
	port := ot.Port

	if host == "" && isFTPType(ot.Type) {
		host = hostFromTargetPath(ot.Path, ot.Type)
	}

	if port == 0 {
		port = defaultPortForType(ot.Type)
	}

	return FTPConfig{
		Host:                      host,
		Username:                  ot.Username,
		Password:                  ot.Password,
		Port:                      port,
		TLS:                       ot.TLS,
		KnownHosts:                ot.KnownHosts,
		InsecureSkipHostKeyVerify: ot.InsecureSkipHostKeyVerify,
	}
}

func isFTPType(targetType string) bool {
	return targetType == "ftp" || targetType == "sftp"
}

func defaultPortForType(targetType string) int {
	if targetType == "sftp" {
		return 22
	}
	return 21
}

func hostFromTargetPath(targetPath, targetType string) string {
	u, err := url.Parse(targetPath)
	if err != nil || u.Host == "" {
		return ""
	}

	host := u.Host
	if strings.Contains(host, ":") {
		return host
	}

	return host + ":" + strconv.Itoa(defaultPortForType(targetType))
}

type OutputConfig []OutputTarget
