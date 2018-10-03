// Copyright 2017 Monax Industries Limited
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package execution

import (
	"fmt"
	"sync"

	"github.com/tendermint/go-amino"
	"github.com/tendermint/tendermint/crypto/tmhash"

	"github.com/hyperledger/burrow/acm"
	"github.com/hyperledger/burrow/acm/state"
	"github.com/hyperledger/burrow/binary"
	"github.com/hyperledger/burrow/crypto"
	"github.com/hyperledger/burrow/execution/exec"
	"github.com/hyperledger/burrow/execution/names"
	"github.com/hyperledger/burrow/genesis"
	"github.com/hyperledger/burrow/permission"
	"github.com/hyperledger/burrow/storage"
	dbm "github.com/tendermint/tendermint/libs/db"
)

const (
	defaultCacheCapacity = 1024
	uint64Length         = 8

	// Prefix under which the versioned merkle state tree resides - tracking previous versions of history
	treePrefix = "m"
	// Prefix under which all non-versioned values reside - either immutable values of references to immutable values
	// that track the current state rather than being part of the history.
	refsPrefix = "r"
)

var (
	// Directly referenced values
	accountKeyFormat = storage.NewMustKeyFormat("a", crypto.AddressLength)
	storageKeyFormat = storage.NewMustKeyFormat("s", crypto.AddressLength, binary.Word256Length)
	nameKeyFormat    = storage.NewMustKeyFormat("n", storage.VariadicSegmentLength)
	// Keys that reference references
	blockRefKeyFormat = storage.NewMustKeyFormat("b", uint64Length)
	txRefKeyFormat    = storage.NewMustKeyFormat("t", uint64Length, uint64Length)
	// Reference keys
	// TODO: implement content-addressing of code and optionally blocks (to allow reference to block to be stored in state tree)
	//codeKeyFormat   = storage.NewMustKeyFormat("c", sha256.Size)
	//blockKeyFormat  = storage.NewMustKeyFormat("b", sha256.Size)
	txKeyFormat     = storage.NewMustKeyFormat("b", tmhash.Size)
	commitKeyFormat = storage.NewMustKeyFormat("x", tmhash.Size)
)

// Implements account and blockchain state
var _ state.IterableReader = &State{}
var _ names.IterableReader = &State{}
var _ Updatable = &writeState{}

type Updatable interface {
	state.Writer
	names.Writer
	AddBlock(blockExecution *exec.BlockExecution) error
}

// Wraps state to give access to writer methods
type writeState struct {
	state *State
}

type CommitID struct {
	Hash binary.HexBytes
	// Height and Version will normally be the same - but it's not clear we should assume this
	Height  uint64
	Version int64
}

// Writers to state are responsible for calling State.Lock() before calling
type State struct {
	// Values not reassigned
	sync.RWMutex
	writeState *writeState
	height     uint64
	db         dbm.DB
	cacheDB    *storage.CacheDB
	tree       *storage.RWTree
	refs       storage.KVStore
	codec      *amino.Codec
}

// Create a new State object
func NewState(db dbm.DB) *State {
	// We collapse all db operations into a single batch committed by save()
	cacheDB := storage.NewCacheDB(db)
	tree := storage.NewRWTree(storage.NewPrefixDB(cacheDB, treePrefix), defaultCacheCapacity)
	refs := storage.NewPrefixDB(cacheDB, refsPrefix)
	s := &State{
		db:      db,
		cacheDB: cacheDB,
		tree:    tree,
		refs:    refs,
		codec:   amino.NewCodec(),
	}
	s.writeState = &writeState{state: s}
	return s
}

// Make genesis state from GenesisDoc and save to DB
func MakeGenesisState(db dbm.DB, genesisDoc *genesis.GenesisDoc) (*State, error) {
	if len(genesisDoc.Validators) == 0 {
		return nil, fmt.Errorf("the genesis file has no validators")
	}

	s := NewState(db)

	// Make accounts state tree
	for _, genAcc := range genesisDoc.Accounts {
		perm := genAcc.Permissions
		acc := &acm.ConcreteAccount{
			Address:     genAcc.Address,
			Balance:     genAcc.Amount,
			Permissions: perm,
		}
		err := s.writeState.UpdateAccount(acc.Account())
		if err != nil {
			return nil, err
		}
	}

	// global permissions are saved as the 0 address
	// so they are included in the accounts tree
	globalPerms := permission.DefaultAccountPermissions
	globalPerms = genesisDoc.GlobalPermissions
	// XXX: make sure the set bits are all true
	// Without it the HasPermission() functions will fail
	globalPerms.Base.SetBit = permission.AllPermFlags

	permsAcc := &acm.ConcreteAccount{
		Address:     acm.GlobalPermissionsAddress,
		Balance:     1337,
		Permissions: globalPerms,
	}
	err := s.writeState.UpdateAccount(permsAcc.Account())
	if err != nil {
		return nil, err
	}

	// We need to save at least once so that readTree points at a non-working-state tree
	_, err = s.writeState.commit()
	if err != nil {
		return nil, err
	}
	return s, nil

}

// Tries to load the execution state from DB, returns nil with no error if no state found
func LoadState(db dbm.DB, hash []byte) (*State, error) {
	s := NewState(db)
	// Get the version associated with this state hash
	commitID := new(CommitID)
	err := s.codec.UnmarshalBinary(s.refs.Get(commitKeyFormat.Key(hash)), commitID)
	if err != nil {
		return nil, fmt.Errorf("could not decode CommitID: %v", err)
	}
	if commitID.Version <= 0 {
		return nil, fmt.Errorf("trying to load state from non-positive version: CommitID: %v", commitID)
	}
	err = s.tree.Load(commitID.Version)
	if err != nil {
		return nil, fmt.Errorf("could not load current version of state tree: CommitID: %v", commitID)
	}
	return s, nil
}

// Perform updates to state whilst holding the write lock, allows a commit to hold the write lock across multiple
// operations while preventing interlaced reads and writes
func (s *State) Update(updater func(up Updatable) error) ([]byte, error) {
	s.Lock()
	defer s.Unlock()
	err := updater(s.writeState)
	if err != nil {
		return nil, err
	}
	return s.writeState.commit()
}

func (ws *writeState) commit() ([]byte, error) {
	// save state at a new version may still be orphaned before we save the version against the hash
	hash, treeVersion, err := ws.state.tree.Save()
	if err != nil {
		return nil, err
	}
	if len(hash) == 0 {
		// Normalise the hash of an empty to tree to the correct hash size
		hash = make([]byte, tmhash.Size)
	}
	// Provide a reference to load this version in the future from the state hash
	commitID := CommitID{
		Hash:    hash,
		Height:  ws.state.height,
		Version: treeVersion,
	}
	bs, err := ws.state.codec.MarshalBinary(commitID)
	if err != nil {
		return nil, fmt.Errorf("could not encode CommitID %v: %v", commitID, err)
	}
	ws.state.refs.Set(commitKeyFormat.Key(hash), bs)
	// Commit the state in cacheDB atomically for this block (synchronous)
	batch := ws.state.db.NewBatch()
	ws.state.cacheDB.Commit(batch)
	batch.WriteSync()
	return hash, err
}

// Returns nil if account does not exist with given address.
func (s *State) GetAccount(address crypto.Address) (acm.Account, error) {
	accBytes := s.tree.Get(accountKeyFormat.Key(address))
	if accBytes == nil {
		return nil, nil
	}
	return acm.Decode(accBytes)
}

func (ws *writeState) UpdateAccount(account acm.Account) error {
	if account == nil {
		return fmt.Errorf("UpdateAccount passed nil account in State")
	}
	encodedAccount, err := account.Encode()
	if err != nil {
		return fmt.Errorf("UpdateAccount could not encode account: %v", err)
	}
	ws.state.tree.Set(accountKeyFormat.Key(account.Address()), encodedAccount)
	return nil
}

func (ws *writeState) RemoveAccount(address crypto.Address) error {
	ws.state.tree.Delete(accountKeyFormat.Key(address))
	return nil
}

func (s *State) IterateAccounts(consumer func(acm.Account) (stop bool)) (stopped bool, err error) {
	it := accountKeyFormat.Iterator(s.tree, nil, nil)
	for it.Valid() {
		account, err := acm.Decode(it.Value())
		if err != nil {
			return true, fmt.Errorf("IterateAccounts could not decode account: %v", err)
		}
		if consumer(account) {
			return true, nil
		}
		it.Next()
	}
	return false, nil
}

func (s *State) GetStorage(address crypto.Address, key binary.Word256) (binary.Word256, error) {
	return binary.LeftPadWord256(s.tree.Get(storageKeyFormat.Key(address, key))), nil
}

func (ws *writeState) SetStorage(address crypto.Address, key, value binary.Word256) error {
	if value == binary.Zero256 {
		ws.state.tree.Delete(storageKeyFormat.Key(address, key))
	} else {
		ws.state.tree.Set(storageKeyFormat.Key(address, key), value.Bytes())
	}
	return nil
}

func (s *State) IterateStorage(address crypto.Address, consumer func(key, value binary.Word256) (stop bool)) (stopped bool, err error) {
	it := storageKeyFormat.Fix(address).Iterator(s.tree, nil, nil)
	for it.Valid() {
		key := it.Key()
		// Note: no left padding should occur unless there is a bug and non-words have been written to this storage tree
		if len(key) != binary.Word256Length {
			return true, fmt.Errorf("key '%X' stored for account %s is not a %v-byte word",
				key, address, binary.Word256Length)
		}
		value := it.Value()
		if len(value) != binary.Word256Length {
			return true, fmt.Errorf("value '%X' stored for account %s is not a %v-byte word",
				key, address, binary.Word256Length)
		}
		if consumer(binary.LeftPadWord256(key), binary.LeftPadWord256(value)) {
			return true, nil
		}
		it.Next()
	}
	return false, nil
}

// State.storage
//-------------------------------------
// Events

// Execution events
func (ws *writeState) AddBlock(be *exec.BlockExecution) error {
	if ws.state.height > 0 && be.Height != ws.state.height+1 {
		return fmt.Errorf("AddBlock received block for height %v but last block height was %v",
			be.Height, ws.state.height)
	}
	ws.state.height = be.Height
	// Index transactions so they can be retrieved by their TxHash
	for i, txe := range be.TxExecutions {
		ws.addTx(txe.TxHash, be.Height, uint64(i))
	}
	bs, err := be.Encode()
	if err != nil {
		return err
	}
	ws.state.refs.Set(blockRefKeyFormat.Key(be.Height), bs)
	return nil
}

func (ws *writeState) addTx(txHash []byte, height, index uint64) {
	ws.state.refs.Set(txKeyFormat.Key(txHash), txRefKeyFormat.Key(height, index))
}

func (s *State) GetTx(txHash []byte) (*exec.TxExecution, error) {
	bs := s.tree.Get(txKeyFormat.Key(txHash))
	if len(bs) == 0 {
		return nil, nil
	}
	height, index := new(uint64), new(uint64)
	txRefKeyFormat.Scan(bs, height, index)
	be, err := s.GetBlock(*height)
	if err != nil {
		return nil, fmt.Errorf("error getting block %v containing tx %X", height, txHash)
	}
	if *index < uint64(len(be.TxExecutions)) {
		return be.TxExecutions[*index], nil
	}
	return nil, fmt.Errorf("retrieved index %v in block %v for tx %X but block only contains %v TxExecutions",
		index, height, txHash, len(be.TxExecutions))
}

func (s *State) GetBlock(height uint64) (*exec.BlockExecution, error) {
	bs := s.tree.Get(blockRefKeyFormat.Key(height))
	if len(bs) == 0 {
		return nil, nil
	}
	return exec.DecodeBlockExecution(bs)
}

func (s *State) GetBlocks(startHeight, endHeight uint64, consumer func(*exec.BlockExecution) (stop bool)) (stopped bool, err error) {
	kf := blockRefKeyFormat
	it := kf.Iterator(s.refs, kf.Suffix(startHeight), kf.Suffix(endHeight))
	for it.Valid() {
		block, err := exec.DecodeBlockExecution(it.Value())
		if err != nil {
			return true, fmt.Errorf("error unmarshalling ExecutionEvent in GetEvents: %v", err)
		}
		if consumer(block) {
			return true, nil
		}
		it.Next()
	}
	return false, nil
}

func (s *State) Hash() []byte {
	s.RLock()
	defer s.RUnlock()
	return s.tree.Hash()
}

// Events
//-------------------------------------
// State.nameReg

var _ names.IterableReader = &State{}

func (s *State) GetName(name string) (*names.Entry, error) {
	entryBytes := s.tree.Get(nameKeyFormat.Key(name))
	if entryBytes == nil {
		return nil, nil
	}

	return names.DecodeEntry(entryBytes)
}

func (ws *writeState) UpdateName(entry *names.Entry) error {
	bs, err := entry.Encode()
	if err != nil {
		return err
	}
	ws.state.tree.Set(nameKeyFormat.Key(entry.Name), bs)
	return nil
}

func (ws *writeState) RemoveName(name string) error {
	ws.state.tree.Delete(nameKeyFormat.Key(name))
	return nil
}

func (s *State) IterateNames(consumer func(*names.Entry) (stop bool)) (stopped bool, err error) {
	it := nameKeyFormat.Iterator(s.tree, nil, nil)
	for it.Valid() {
		entry, err := names.DecodeEntry(it.Value())
		if err != nil {
			return true, fmt.Errorf("State.IterateNames() could not iterate over names: %v", err)
		}
		if consumer(entry) {
			return true, nil
		}
		it.Next()
	}
	return false, nil
}

// Creates a copy of the database to the supplied db
func (s *State) Copy(db dbm.DB) (*State, error) {
	stateCopy := NewState(db)
	s.tree.IterateRange(nil, nil, true, func(key, value []byte) bool {
		stateCopy.tree.Set(key, value)
		return false
	})
	_, err := stateCopy.writeState.commit()
	if err != nil {
		return nil, err
	}
	return stateCopy, nil
}
