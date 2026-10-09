package contracts

import "github.com/Liapoldus/forms-db/contracts/policy"

func Limits() (policy.ResourceLimits, error) { return policy.Limits(), nil }
