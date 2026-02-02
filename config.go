package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Upstream UpstreamConfig `yaml:"upstream"`
	Local    LocalConfig    `yaml:"local"`
	Cache    CacheConfig    `yaml:"cache"`
}

type ServerConfig struct {
	Address  string `yaml:"addr"`
	Port     string `yaml:"port"`
	Compress bool   `yaml:"compress"`
}

type UpstreamConfig struct {
	Mode          string    `yaml:"mode"`
	Timeout       int       `yaml:"timeout"`
	KeepAlive     int       `yaml:"keep_alive"`
	BufferSize    int       `yaml:"buffer_size"`
	PoolSize      int       `yaml:"pool_size"`
	SkipTLSVerify bool      `yaml:"skip_tls_verify"`
	Domain        string    `yaml:"domain"`
	Addresses     []string  `yaml:"addresses"`
	DoH           DoHConfig `yaml:"doh"`
}

type DoHConfig struct {
	QueryPath string     `yaml:"query_path"`
	Idle      IdleConfig `yaml:"idle"`
}

type IdleConfig struct {
	MaxConnection        int `yaml:"max_conn"`
	MaxConnectionPerHost int `yaml:"max_per_host"`
}

type LocalConfig struct {
	Enable        bool           `yaml:"enable"`
	UseHostsFile  bool           `yaml:"use_hosts_file"`
	StaticRecords []StaticRecord `yaml:"static_records"`
}

type StaticRecord struct {
	Domain string `yaml:"domain"`
	IP     string `yaml:"ip"`
}

type CacheConfig struct {
	Size   int `yaml:"size"`
	MinTTL int `yaml:"min_ttl"`
	NegTTL int `yaml:"neg_ttl"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	config.Server.Address = "0.0.0.0"
	config.Server.Port = "5353"
	config.Server.Compress = true

	config.Upstream.Mode = "udp"
	config.Upstream.Timeout = 5
	config.Upstream.KeepAlive = 60
	config.Upstream.BufferSize = 4096
	config.Upstream.PoolSize = 100
	config.Upstream.SkipTLSVerify = true

	config.Upstream.DoH.QueryPath = "/dns-query"
	config.Upstream.DoH.Idle.MaxConnection = 100
	config.Upstream.DoH.Idle.MaxConnectionPerHost = 20

	config.Local.Enable = false
	config.Local.UseHostsFile = false

	config.Cache.Size = 10000
	config.Cache.MinTTL = 60
	config.Cache.NegTTL = 1

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
