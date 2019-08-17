package contexts

import (
	"fmt"
	"math/big"

	"github.com/hyperledger/burrow/acm"
	"github.com/hyperledger/burrow/acm/acmstate"
	"github.com/hyperledger/burrow/acm/validator"
	"github.com/hyperledger/burrow/crypto"
	"github.com/hyperledger/burrow/execution/errors"
	"github.com/hyperledger/burrow/execution/exec"
	"github.com/hyperledger/burrow/genesis/spec"
	"github.com/hyperledger/burrow/logging"
	"github.com/hyperledger/burrow/permission"
	"github.com/hyperledger/burrow/txs/payload"
)

type GovernanceContext struct {
	StateWriter  acmstate.ReaderWriter
	ValidatorSet validator.ReaderWriter
	Logger       *logging.Logger
	tx           *payload.GovTx
	txe          *exec.TxExecution
}

// GovTx provides a set of TemplateAccounts and GovernanceContext tries to alter the chain state to match the
// specification given
func (ctx *GovernanceContext) Execute(txe *exec.TxExecution, p payload.Payload) error {
	var ok bool
	ctx.txe = txe
	ctx.tx, ok = p.(*payload.GovTx)
	if !ok {
		return fmt.Errorf("payload must be NameTx, but is: %v", txe.Envelope.Tx.Payload)
	}
	// Nothing down with any incoming funds at this point
	accounts, _, err := getInputs(ctx.StateWriter, ctx.tx.Inputs)
	if err != nil {
		return err
	}

	// ensure all inputs have root permissions
	err = allHavePermission(ctx.StateWriter, permission.Root, accounts, ctx.Logger)
	if err != nil {
		return errors.Wrap(err, "at least one input lacks permission for GovTx")
	}

	for _, i := range ctx.tx.Inputs {
		txe.Input(i.Address, nil)
	}

	for _, update := range ctx.tx.AccountUpdates {
		err := VerifyIdentity(ctx.StateWriter, update)
		if err != nil {
			return fmt.Errorf("GovTx: %v", err)
		}
		account, err := getOrMakeOutput(ctx.StateWriter, accounts, *update.Address, ctx.Logger)
		if err != nil {
			return err
		}
		governAccountEvent, err := ctx.UpdateAccount(account, update)
		if err != nil {
			txe.GovernAccount(governAccountEvent, errors.AsException(err))
			return err
		}
		txe.GovernAccount(governAccountEvent, nil)
	}
	return nil
}

func (ctx *GovernanceContext) UpdateAccount(account *acm.Account, update *spec.TemplateAccount) (ev *exec.GovernAccountEvent, err error) {
	ev = &exec.GovernAccountEvent{
		AccountUpdate: update,
	}
	if update.Balances().HasNative() {
		account.Balance = update.Balances().GetNative(0)
	}
	if update.Balances().HasPower() {
		if update.PublicKey == nil {
			err = fmt.Errorf("updateAccount should have PublicKey by this point but appears not to for "+
				"template account: %v", update)
			return
		}
		power := new(big.Int).SetUint64(update.Balances().GetPower(0))
		_, err := ctx.ValidatorSet.SetPower(*update.PublicKey, power)
		if err != nil {
			return ev, err
		}
	}
	if update.Code != nil {
		account.EVMCode = *update.Code
		if err != nil {
			return ev, err
		}
	}
	perms := account.Permissions
	if len(update.Permissions) > 0 {
		perms.Base, err = permission.BasePermissionsFromStringList(update.Permissions)
		if err != nil {
			return
		}
	}
	if len(update.Roles) > 0 {
		perms.Roles = update.Roles
	}
	account.Permissions = perms
	if err != nil {
		return
	}
	err = ctx.StateWriter.UpdateAccount(account)
	return
}

func VerifyIdentity(sw acmstate.ReaderWriter, account *spec.TemplateAccount) (err error) {
	if account.Address == nil && account.PublicKey == nil {
		// We do not want to generate a key
		return fmt.Errorf("could not execute Tx since account template %v contains neither "+
			"address or public key", account)
	}
	if account.PublicKey == nil {
		account.PublicKey, err = MaybeGetPublicKey(sw, *account.Address)
		if err != nil {
			return err
		}
	}
	// Check address
	if account.PublicKey != nil {
		address := account.PublicKey.GetAddress()
		if account.Address != nil && address != *account.Address {
			return fmt.Errorf("supplied public key %v whose address %v does not match %v provided by"+
				"GovTx", account.PublicKey, address, account.Address)
		}
		account.Address = &address
	} else if account.Balances().HasPower() {
		// If we are updating power we will need the key
		return fmt.Errorf("must be provided with public key when updating validator power")
	}
	return nil
}

func MaybeGetPublicKey(sw acmstate.ReaderWriter, address crypto.Address) (*crypto.PublicKey, error) {
	// First try state in case chain has received input previously
	acc, err := sw.GetAccount(address)
	if err != nil {
		return nil, err
	}
	if acc != nil && acc.PublicKey.IsSet() {
		publicKey := acc.PublicKey
		return &publicKey, nil
	}
	return nil, nil
}
