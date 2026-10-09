package config

import "github.com/Liapoldus/forms-db/contracts/policy"

func DSNSecretDescription() (string, bool)  { return policy.Secrets().DSNDescription, true }
func DSNSecretGrantPurpose() (string, bool) { return policy.Secrets().DSNGrantPurpose, true }
