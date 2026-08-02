package config

type FTPConfig struct {
	Host     string `yaml:"host"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Port     int    `yaml:"port"` // Optional, default 21 for FTP, 22 for SFTP

	// SFTP host key verification
	KnownHosts                string `yaml:"known-hosts"`                          // Path to a known_hosts file; falls back to ~/.ssh/known_hosts and /etc/ssh/ssh_known_hosts
	InsecureSkipHostKeyVerify bool   `yaml:"insecure-skip-host-key-verification"` // Explicitly disables host key verification (vulnerable to MITM)
}
