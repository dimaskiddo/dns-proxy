package config

type Config struct {
	Server        ServerConfig        `yaml:"server" mapstructure:"server"`
	Upstream      UpstreamConfig      `yaml:"upstream" mapstructure:"upstream"`
	Cache         CacheConfig         `yaml:"cache" mapstructure:"cache"`
	BogusNXDomain BogusNXDomainConfig `yaml:"bogus-nxdomain" mapstructure:"bogus-nxdomain"`
	EDNS          EDNSConfig          `yaml:"edns" mapstructure:"edns"`
	Local         LocalConfig         `yaml:"local" mapstructure:"local"`
	Forwarder     ForwarderConfig     `yaml:"forwarder" mapstructure:"forwarder"`
}

type ServerConfig struct {
	Listen   []string `yaml:"listen" mapstructure:"listen"`
	Compress bool     `yaml:"compress" mapstructure:"compress"`
}

type UpstreamConfig struct {
	Mode          string    `yaml:"mode" mapstructure:"mode"`
	Timeout       int       `yaml:"timeout" mapstructure:"timeout"`
	KeepAlive     int       `yaml:"keep_alive" mapstructure:"keep_alive"`
	BufferSize    int       `yaml:"buffer_size" mapstructure:"buffer_size"`
	PoolSize      int       `yaml:"pool_size" mapstructure:"pool_size"`
	MaxAttempts   int       `yaml:"max_attempts" mapstructure:"max_attempts"`
	DisableIPv6   bool      `yaml:"disable_ipv6" mapstructure:"disable_ipv6"`
	SkipTLSVerify bool      `yaml:"skip_tls_verify" mapstructure:"skip_tls_verify"`
	Domain        string    `yaml:"domain" mapstructure:"domain"`
	Addresses     []string  `yaml:"addresses" mapstructure:"addresses"`
	DoH           DoHConfig `yaml:"doh" mapstructure:"doh"`
}

type DoHConfig struct {
	QueryPath string     `yaml:"query_path" mapstructure:"query_path"`
	Idle      IdleConfig `yaml:"idle" mapstructure:"idle"`
}

type IdleConfig struct {
	MaxConnection        int `yaml:"max_conn" mapstructure:"max_conn"`
	MaxConnectionPerHost int `yaml:"max_per_host" mapstructure:"max_per_host"`
}

type CacheConfig struct {
	Size   int `yaml:"size" mapstructure:"size"`
	Shards int `yaml:"shards" mapstructure:"shards"`
	MinTTL int `yaml:"min_ttl" mapstructure:"min_ttl"`
	NegTTL int `yaml:"neg_ttl" mapstructure:"neg_ttl"`
}

type BogusNXDomainConfig struct {
	Enable bool     `yaml:"enable" mapstructure:"enable"`
	IPs    []string `yaml:"ips" mapstructure:"ips"`
}

type EDNSConfig struct {
	Enable   bool `yaml:"enable" mapstructure:"enable"`
	IPv4Mask int  `yaml:"ipv4_mask" mapstructure:"ipv4_mask"`
	IPv6Mask int  `yaml:"ipv6_mask" mapstructure:"ipv6_mask"`
}

type LocalConfig struct {
	Enable          bool           `yaml:"enable" mapstructure:"enable"`
	UseHostsFile    bool           `yaml:"use_hosts_file" mapstructure:"use_hosts_file"`
	CustomHostsFile string         `yaml:"custom_hosts_file" mapstructure:"custom_hosts_file"`
	IncludeFiles    []string       `yaml:"include_files" mapstructure:"include_files"`
	StaticRecords   []StaticRecord `yaml:"static_records" mapstructure:"static_records"`
}

type StaticRecord struct {
	Domain string `yaml:"domain" mapstructure:"domain"`
	IP     string `yaml:"ip" mapstructure:"ip"`
}

type ForwarderConfig struct {
	Enable       bool            `yaml:"enable" mapstructure:"enable"`
	IncludeFiles []string        `yaml:"include_files" mapstructure:"include_files"`
	Rules        []ForwarderRule `yaml:"rules" mapstructure:"rules"`
}

type ForwarderRule struct {
	Domain    string   `yaml:"domain" mapstructure:"domain"`
	Upstreams []string `yaml:"upstreams" mapstructure:"upstreams"`
}

// setDefaultConfig populates cfg with the same defaults LoadConfig used to
// assign before unmarshalling on top of them.
func setDefaultConfig(cfg *Config) {
	cfg.Server.Listen = []string{"0.0.0.0:5353"}
	cfg.Server.Compress = true

	cfg.Upstream.Timeout = 10
	cfg.Upstream.KeepAlive = 60
	cfg.Upstream.BufferSize = 4096
	cfg.Upstream.PoolSize = 100
	cfg.Upstream.MaxAttempts = 3
	cfg.Upstream.DisableIPv6 = false
	cfg.Upstream.SkipTLSVerify = true
	cfg.Upstream.Mode = "udp"

	cfg.Upstream.DoH.QueryPath = "/dns-query"
	cfg.Upstream.DoH.Idle.MaxConnection = 100
	cfg.Upstream.DoH.Idle.MaxConnectionPerHost = 20

	cfg.Cache.Size = 10000
	cfg.Cache.Shards = 256
	cfg.Cache.MinTTL = 60
	cfg.Cache.NegTTL = 1

	cfg.BogusNXDomain.Enable = false

	cfg.EDNS.Enable = false
	cfg.EDNS.IPv4Mask = 24
	cfg.EDNS.IPv6Mask = 56

	cfg.Local.Enable = false
	cfg.Local.UseHostsFile = false

	cfg.Forwarder.Enable = false
}
