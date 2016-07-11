![License: Apache v2.0](https://badges.fyi/github/license/Jimdo/yaml-vault)

# Jimdo / yaml-vault

`yaml-vault` is a small utility to import data from a YAML file to Vault or export keys from Vault into a YAML file.

## Usage

```bash
# cat vault.yaml
keys:
  secret/integration/test:
    bar: foo
    foo: bar

# yaml-vault --import -f vault.yaml

# vault read secret/integration/test
Key                     Value
---                     -----
refresh_interval        2592000
bar                     foo
foo                     bar

```
