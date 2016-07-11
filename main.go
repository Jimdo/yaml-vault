package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/Luzifer/rconfig"
	"github.com/hashicorp/vault/api"
	"github.com/mitchellh/go-homedir"
)

var (
	cfg = struct {
		File           string   `flag:"file,f" default:"vault.yaml" description:"File to import from / export to"`
		Import         bool     `flag:"import" default:"false" description:"Enable importing data into Vault"`
		Export         bool     `flag:"export" default:"false" description:"Enable exporting data from Vault"`
		ExportPaths    []string `flag:"export-paths" default:"secret" description:"Which paths to export"`
		VaultAddress   string   `flag:"vault-addr" env:"VAULT_ADDR" default:"https://127.0.0.1:8200" description:"Vault API address"`
		VaultToken     string   `flag:"vault-token" env:"VAULT_TOKEN" vardefault:"vault-token" description:"Specify a token to use instead of app-id auth"`
		VersionAndExit bool     `flag:"version" default:"false" description:"Print program version and exit"`
		Verbose        bool     `flag:"verbose,v" default:"false" description:"Print verbose output"`
	}{}

	version = "dev"
)

type importFile struct {
	Keys map[string]map[string]interface{}
}

type execFunction func(*api.Client) error

func debug(format string, v ...interface{}) {
	if cfg.Verbose {
		log.Printf(format, v...)
	}
}

func vaultTokenFromDisk() string {
	vf, err := homedir.Expand("~/.vault-token")
	if err != nil {
		return ""
	}

	data, err := ioutil.ReadFile(vf)
	if err != nil {
		return ""
	}

	return string(data)
}

func init() {
	rconfig.SetVariableDefaults(map[string]string{
		"vault-token": vaultTokenFromDisk(),
	})
	rconfig.Parse(&cfg)

	if cfg.VersionAndExit {
		fmt.Printf("vault2env %s\n", version)
		os.Exit(0)
	}

	if cfg.VaultToken == "" {
		log.Fatalf("[ERR] You need to set vault-token")
	}

	if cfg.File == "" {
		log.Fatalf("[ERR] You need to specify a file")
	}

	if cfg.Export == cfg.Import {
		log.Fatalf("[ERR] You need to either import or export")
	}

	if _, err := os.Stat(cfg.File); (err == nil && cfg.Export) || (err != nil && cfg.Import) {
		if cfg.Export {
			log.Fatalf("[ERR] Output file exists, stopping now.")
		}
		log.Fatalf("[ERR] Input file does not exist, stopping now.")
	}
}

func main() {
	client, err := api.NewClient(&api.Config{
		Address: cfg.VaultAddress,
	})
	if err != nil {
		log.Fatalf("Unable to create client: %s", err)
	}

	client.SetToken(cfg.VaultToken)

	var ex execFunction

	if cfg.Export {
		ex = exportFromVault
	} else {
		ex = importToVault
	}

	if err = ex(client); err != nil {
		log.Fatalf("[ERR] %s", err)
	}
}

func exportFromVault(client *api.Client) error {
	out := importFile{
		Keys: make(map[string]map[string]interface{}),
	}

	for _, path := range cfg.ExportPaths {
		if path[0] == '/' {
			path = path[1:]
		}

		if !strings.HasSuffix(path, "/") {
			path = path + "/"
		}

		if err := readRecurse(client, path, &out); err != nil {
			return err
		}
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(cfg.File, data, 0600)
}

func readRecurse(client *api.Client, path string, out *importFile) error {
	if strings.HasSuffix(path, "/") {
		secret, err := client.Logical().List(path)
		if err != nil {
			return fmt.Errorf("Error reading %s: %s", path, err)
		}

		if secret != nil && secret.Data["keys"] != nil {
			for _, k := range secret.Data["keys"].([]interface{}) {
				if err := readRecurse(client, path+k.(string), out); err != nil {
					return err
				}
			}
			return nil
		}
	}

	secret, err := client.Logical().Read(path)
	if err != nil {
		return err
	}

	if secret == nil {
		return fmt.Errorf("Unable to read %s: %#v", path, secret)
	}

	out.Keys[path] = secret.Data
	debug("Successfully read data from key '%s'", path)

	return nil
}

func importToVault(client *api.Client) error {
	keysRaw, err := ioutil.ReadFile(cfg.File)
	if err != nil {
		return err
	}

	keysRaw, err = parseImportFile(keysRaw)
	if err != nil {
		return err
	}

	var keys importFile
	if err := yaml.Unmarshal(keysRaw, &keys); err != nil {
		return err
	}

	for key, data := range keys.Keys {
		if _, err := client.Logical().Write(key, data); err != nil {
			return err
		}
		debug("Successfully wrote data to key '%s'", key)
	}

	return nil
}

func parseImportFile(in []byte) (out []byte, err error) {
	funcMap := template.FuncMap{
		"env": func(name string, v ...string) string {
			defaultValue := ""
			if len(v) > 0 {
				defaultValue = v[0]
			}
			if value, ok := os.LookupEnv(name); ok {
				return value
			}
			return defaultValue
		},
	}

	t, err := template.New("input file").Funcs(funcMap).Parse(string(in))
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer([]byte{})
	return buf.Bytes(), t.Execute(buf, nil)
}
