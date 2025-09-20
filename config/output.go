package config

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
	port := ot.Port
	if port == 0 {
		// Standard-Port basierend auf Typ setzen
		if ot.Type == "sftp" {
			port = 22
		} else {
			port = 21
		}
	}
	return FTPConfig{
		Host:     ot.Host,
		Username: ot.Username,
		Password: ot.Password,
		Port:     port,
	}
}

type OutputConfig []OutputTarget
