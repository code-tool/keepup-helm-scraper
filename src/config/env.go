package config

import (
	"log"
	"os"
	"reflect"

	"github.com/joho/godotenv"
)

type EnvConfig struct {
	APP_ENV      string
	API_URL      string
	API_TOKEN    string
	CLUSTER_NAME string
	RULES_FILE   string
}

var config *EnvConfig

func GetEnvConfig() EnvConfig {
	return *config
}

func loadEnvFile() {
	log.Println("Loading .env file.")
	err := godotenv.Load(".env")
	if err != nil {
		panic("Error loading .env file.")
	}
}

func init() {
	config = &EnvConfig{}
	_, found := os.LookupEnv("APP_ENV")
	if !found {
		loadEnvFile()
	}
	_, found = os.LookupEnv("RULES_FILE")
	if !found {
		os.Setenv("RULES_FILE", "./keepup-detection.yaml")
	}
	refl := reflect.ValueOf(config).Elem()
	numFields := refl.NumField()
	for i := 0; i < numFields; i++ {
		envName := refl.Type().Field(i).Name
		envVal, foud := os.LookupEnv(envName)
		if !foud {
			panic("Environment [" + envName + "] not found.")
		}
		refl.Field(i).SetString(envVal)
	}
}
