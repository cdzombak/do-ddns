package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"do-ddns/server/app"
	"do-ddns/server/cache"
	"do-ddns/server/handler"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	_ "github.com/joho/godotenv/autoload"

	"do-ddns/server/digitalocean"
)

func main() {
	appEnv := app.Env{}
	appEnv.UpdateCache = &cache.DNSUpdateCache{}
	appEnv.Decoder = schema.NewDecoder()

	port := os.Getenv("PORT")
	if port == "" {
		log.Println("environment variable 'PORT' is missing; defaulting to 7001")
		port = "7001"
	}

	doAPIKey := mustGetenv("DO_API_KEY")
	appEnv.DOAPI = &digitalocean.APIClient{}
	if err := appEnv.DOAPI.SetAPIKey(doAPIKey); err != nil {
		log.Fatalf("failed to initialize DigitalOcean API client: %s\n", err.Error())
	}

	domainsConfigPath := mustGetenv("DOMAINS_CONFIG_PATH")
	if err := appEnv.ReadDomainsConfig(domainsConfigPath); err != nil {
		log.Fatalf("couldn't load config file '%s': %s\n", domainsConfigPath, err.Error())
	}

	sigUSR2Chan := make(chan os.Signal, 1)
	signal.Notify(sigUSR2Chan, syscall.SIGUSR2)
	go func(){
		for _ = range sigUSR2Chan {
			log.Println("got SIGUSR2; reloading config file")
			if err := appEnv.ReadDomainsConfig(domainsConfigPath); err != nil {
				log.Println(err.Error())
				log.Println("continuing with unchanged config")
			}
		}
	}()

	router := mux.NewRouter().StrictSlash(false)
	router.Methods("GET").Path("/ping").Handler(app.Handler{E: &appEnv, H: handler.Ping})
	router.Methods("GET").Path("/v3/update").Handler(app.Handler{E: &appEnv, H: handler.DynDnsApiUpdate})
	router.Methods("GET").Path("/nic/update").Handler(app.Handler{E: &appEnv, H: handler.DynDnsApiUpdate})
	router.Methods("POST").Path("/").Handler(app.Handler{E: &appEnv, H: handler.PostUpdate})
	log.Printf("server is listening on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// mustGetenv returns the value of the environment variable with the given name, or exits
// with an error if the variable is empty.
func mustGetenv(key string) string {
	retv := os.Getenv(key)
	if retv == "" {
		log.Fatalf("environment variable '%s' is missing\n", key)
	}
	return retv
}
