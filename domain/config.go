package domain

// # App configuration
// app:
//   port: 9001

// # Redis configuration
// redis:
//   host: localhost
//   port: 6379
//   db: 0
//   password: ""

type Config struct {
	App   AppConfig   `yaml:"app"`
	Redis RedisConfig `yaml:"redis"`
}

type AppConfig struct {
	Port int `yaml:"port"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	DB       int    `yaml:"db"`
	Password string `yaml:"password"`
}
