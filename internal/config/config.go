package config

import (
	"log"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	ListenAddr         string `env:"WA_APP_LISTEN_ADDR" envDefault:":50091"`
	DashboardHTTPAddr  string `env:"WA_APP_DASHBOARD_HTTP_ADDR" envDefault:":8080"`
	DashboardStaticDir string `env:"WA_APP_DASHBOARD_STATIC_DIR" envDefault:"/app/dashboard/wa"`
	CommonProxy        string `env:"WA_COMMON_PROXY"`
	NumberProbeProxy   string `env:"WA_NUMBER_PROBE_PROXY"`
	RegistrationProxy  string `env:"WA_REGISTRATION_PROXY"`
	PGDSN              string `env:"WA_APP_PG_DSN"`
	RedisURL           string `env:"WA_APP_REDIS_URL"`
	DataDir            string `env:"WA_APP_DATA_DIR" envDefault:"/var/lib/wa-app"`
}

func Load() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("load wa-app config: %v", err)
	}
	return cfg
}
