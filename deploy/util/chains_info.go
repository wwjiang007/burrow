package util

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hyperledger/burrow/deploy/def"
	"github.com/hyperledger/burrow/logging"

	"encoding/json"

	"github.com/elgs/gojq"
	"github.com/hyperledger/burrow/acm/validator"
)

func GetBlockHeight(client *def.Client, logger *logging.Logger) (latestBlockHeight uint64, err error) {
	stat, err := client.Status(logger)
	if err != nil {
		return 0, err
	}
	return stat.SyncInfo.LatestBlockHeight, nil
}

func AccountsInfo(account, field string, client *def.Client, logger *logging.Logger) (string, error) {
	address, err := client.ParseAddress(account, logger)
	if err != nil {
		return "", err
	}
	acc, err := client.GetAccount(address)
	if err != nil {
		return "", err
	}
	if acc == nil {
		return "", fmt.Errorf("Account %s does not exist", account)
	}

	bs, err := json.Marshal(acc)
	if err != nil {
		return "", err
	}
	jq, err := gojq.NewStringQuery(string(bs))
	if err == nil {
		logger.InfoMsg("Attempting jq query")
		res, err := jq.Query(field)
		if err == nil {
			return fmt.Sprintf("%v", res), nil
		} else {
			logger.TraceMsg("Got error from jq query: %v trying legacy query (probably fine)...", "error", fmt.Sprintf("%v", err))
		}
	}

	var s string
	if strings.Contains(field, "permissions") {
		fields := strings.Split(field, ".")

		if len(fields) > 1 {
			switch fields[1] {
			case "roles":
				s = strings.Join(acc.Permissions.Roles, ",")
			case "base", "perms":
				s = strconv.Itoa(int(acc.Permissions.Base.Perms))
			case "set":
				s = strconv.Itoa(int(acc.Permissions.Base.SetBit))
			}
		}
	} else if field == "balance" {
		s = itoaU64(acc.Balance)
	}

	if err != nil {
		return "", err
	}

	return s, nil
}

func NamesInfo(name, field string, client *def.Client, logger *logging.Logger) (string, error) {
	entry, err := client.GetName(name, logger)
	if err != nil {
		return "", err
	}

	switch strings.ToLower(field) {
	case "name":
		return name, nil
	case "owner":
		return entry.Owner.String(), nil
	case "data":
		return entry.Data, nil
	case "expires":
		return itoaU64(entry.Expires), nil
	default:
		return "", fmt.Errorf("Field %s not recognized", field)
	}
}

func ValidatorsInfo(query string, client *def.Client, logger *logging.Logger) (interface{}, error) {
	// Currently there is no notion of 'unbonding validators' we can revisit what should go here or whether this deserves
	// to exist as a job
	validatorSet, err := client.GetValidatorSet(logger)
	if err != nil {
		return nil, err
	}
	// Yes, this feels a bit silly, but it is the easiest way to get the generic map of slice object that gojq needs
	// mapstructure is not able to do this it would seem.
	bs, err := json.Marshal(map[string]interface{}{
		"Height": validatorSet.Height,
		"Set":    validatorMap(validatorSet.Set),
	})
	if err != nil {
		return nil, err
	}
	jq, err := gojq.NewStringQuery(string(bs))
	if err != nil {
		return nil, err
	}
	return jq.Query(query)
}

func validatorMap(vs []*validator.Validator) map[string]interface{} {
	set := validator.UnpersistSet(vs)
	vsMap := make(map[string]interface{}, len(vs))
	vsMap["TotalPower"] = set.TotalPower().String()
	vsMap["String"] = set.String()
	for _, v := range vs {
		vsMap[v.GetPublicKey().GetAddress().String()] = v
	}
	return vsMap
}

func itoaU64(i uint64) string {
	return strconv.FormatUint(i, 10)
}
