package config

import (
	"net/url"
	"strings"
)

type OutputTarget struct {
	Path string `yaml:"path"`
	Type string `yaml:"type"`

	// S3-spezifische Konfiguration
	Endpoint  string `yaml:"endpoint,omitempty"`
	AccessKey string `yaml:"access-key,omitempty"`
	SecretKey string `yaml:"secret-key,omitempty"`
	SSL       *bool  `yaml:"ssl,omitempty"`
	Region    string `yaml:"region,omitempty"`

	// FTP/SFTP-spezifische Konfiguration
	Host     string `yaml:"host,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
	Port     int    `yaml:"port,omitempty"`
}

// GetS3Config extrahiert die S3-Konfiguration aus dem OutputTarget
func (ot *OutputTarget) GetS3Config() S3Config {
	ssl := true // Standard-Wert
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

// GetFTPConfig extrahiert die FTP-Konfiguration aus dem OutputTarget
func (ot *OutputTarget) GetFTPConfig() FTPConfig {
	host := ot.Host
	port := ot.Port

	// Falls kein Host explizit gesetzt ist, versuche ihn aus der URL zu extrahieren
	if host == "" && (ot.Type == "ftp" || ot.Type == "sftp") {
		if u, err := url.Parse(ot.Path); err == nil && u.Host != "" {
			host = u.Host
			// Falls kein Port in der URL angegeben ist, Standard-Port setzen
			if !strings.Contains(host, ":") {
				if ot.Type == "sftp" {
					host += ":22"
				} else {
					host += ":21"
				}
			}
		}
	}

	if port == 0 {
		// Standard-Port basierend auf Typ setzen
		if ot.Type == "sftp" {
			port = 22
		} else {
			port = 21
		}
	}
	return FTPConfig{
		Host:     host,
		Username: ot.Username,
		Password: ot.Password,
		Port:     port,
	}
}

type OutputConfig []OutputTarget
