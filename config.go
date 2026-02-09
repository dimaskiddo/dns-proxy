package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Upstream  UpstreamConfig  `yaml:"upstream"`
	Local     LocalConfig     `yaml:"local"`
	Forwarder ForwarderConfig `yaml:"forwarder"`
	Cache     CacheConfig     `yaml:"cache"`
	EDNS      EDNSConfig      `yaml:"edns"`
}

type ServerConfig struct {
	Listen   []string `yaml:"listen"`
	Compress bool     `yaml:"compress"`
}

type UpstreamConfig struct {
	Mode          string    `yaml:"mode"`
	Timeout       int       `yaml:"timeout"`
	KeepAlive     int       `yaml:"keep_alive"`
	BufferSize    int       `yaml:"buffer_size"`
	PoolSize      int       `yaml:"pool_size"`
	MaxAttempts   int       `yaml:"max_attempts"`
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
	IncludeFiles  []string       `yaml:"include_files"`
	StaticRecords []StaticRecord `yaml:"static_records"`
}

type StaticRecord struct {
	Domain string `yaml:"domain"`
	IP     string `yaml:"ip"`
}

type ForwarderConfig struct {
	Enable       bool            `yaml:"enable"`
	IncludeFiles []string        `yaml:"include_files"`
	Rules        []ForwarderRule `yaml:"rules"`
}

type ForwarderRule struct {
	Domain    string   `yaml:"domain"`
	Upstreams []string `yaml:"upstreams"`
}

type CacheConfig struct {
	Size   int `yaml:"size"`
	Shards int `yaml:"shards"`
	MinTTL int `yaml:"min_ttl"`
	NegTTL int `yaml:"neg_ttl"`
}

type EDNSConfig struct {
	Enable   bool `yaml:"enable"`
	IPv4Mask int  `yaml:"ipv4_mask"`
	IPv6Mask int  `yaml:"ipv6_mask"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	configDir := filepath.Dir(filename)

	config := &Config{}
	config.Server.Listen = []string{"0.0.0.0:5353"}
	config.Server.Compress = true

	config.Upstream.Mode = "udp"
	config.Upstream.Timeout = 5
	config.Upstream.KeepAlive = 60
	config.Upstream.BufferSize = 4096
	config.Upstream.PoolSize = 100
	config.Upstream.MaxAttempts = 3
	config.Upstream.SkipTLSVerify = true

	config.Upstream.DoH.QueryPath = "/dns-query"
	config.Upstream.DoH.Idle.MaxConnection = 100
	config.Upstream.DoH.Idle.MaxConnectionPerHost = 20

	config.Local.Enable = false
	config.Local.UseHostsFile = false

	config.Forwarder.Enable = false

	config.Cache.Size = 10000
	config.Cache.Shards = 256
	config.Cache.MinTTL = 60
	config.Cache.NegTTL = 1

	config.EDNS.Enable = true
	config.EDNS.IPv4Mask = 24
	config.EDNS.IPv6Mask = 56

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	localFiles := parseIncludeFiles(configDir, config.Local.IncludeFiles)
	for _, file := range localFiles {
		subData, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var tempLocal LocalConfig
		if err := yaml.Unmarshal(subData, &tempLocal); err == nil {
			if len(tempLocal.StaticRecords) > 0 {
				config.Local.StaticRecords = append(config.Local.StaticRecords, tempLocal.StaticRecords...)
			}
		}
	}

	forwarderFiles := parseIncludeFiles(configDir, config.Forwarder.IncludeFiles)
	for _, file := range forwarderFiles {
		subData, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var tempForwarder ForwarderConfig
		if err := yaml.Unmarshal(subData, &tempForwarder); err == nil {
			if len(tempForwarder.Rules) > 0 {
				config.Forwarder.Rules = append(config.Forwarder.Rules, tempForwarder.Rules...)
			}
		}
	}

	return config, nil
}
