package ops

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/polyspec/flowmark/packages/go/catalog"
	"github.com/polyspec/orm/packages/go/engine/dbspec"
	"github.com/polyspec/orm/packages/go/orm"
)

// sealed returns values with each value of an aes column encrypted, as a write of orm's client stores it (spec 30 Keys):
// with the current key of the keyring of the process, that version in the version column, and the blind index of the
// column following its value. orm's rules hold: the version is the writer's, and an update of encrypted data assigns every
// aes column, since one version describes the row. A write of an aes column in a process without a keyring is CONFIG: it
// would store the plain text. insert is false for an update.
func (c *Corpus) sealed(tb *catalog.Table, insert bool, values map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if tb.AESVersion != "" {
		if _, assigned := values[tb.AESVersion]; assigned {
			return nil, errf(codeIRInvalid, "%s is managed by the AES writer", tb.AESVersion)
		}
	}
	var assigned, missing []string
	for _, col := range tb.Columns {
		if !slices.Contains(tb.Codecs[col.Name], "aes") {
			continue
		}
		if _, ok := values[col.Name]; ok {
			assigned = append(assigned, col.Name)
		} else {
			missing = append(missing, col.Name)
		}
	}
	if len(assigned) == 0 && (!insert || tb.AESVersion == "") {
		return values, nil
	}
	keys := c.cat.Keys
	if keys == nil {
		if len(assigned) == 0 {
			return nil, errf(codeConfig, "the table %s records a key version and this process has no keyring: set %s", tb.Name, catalog.KeyringVariable)
		}
		return nil, errf(codeConfig, "the table %s has the aes column %s and this process has no keyring: set %s", tb.Name, assigned[0], catalog.KeyringVariable)
	}
	if !insert && len(missing) > 0 {
		return nil, errf(codeIRInvalid, "AES update must assign every AES column; missing %s", missing[0])
	}
	secret, _ := keys.Key(keys.Current)
	out := make(map[string]json.RawMessage, len(values)+len(assigned)+1)
	for name, v := range values {
		out[name] = v
	}
	if tb.AESVersion != "" {
		out[tb.AESVersion] = json.RawMessage(strconv.FormatInt(int64(keys.Current), 10))
	}
	for _, column := range assigned {
		if err := seal(tb, keys, secret, column, values[column], out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// seal writes into out the stored form of the plain value v of the aes column and of its blind index.
func seal(tb *catalog.Table, keys *catalog.Keys, secret, column string, v json.RawMessage, out map[string]json.RawMessage) error {
	index, indexed := tb.BlindIndexes[column]
	var plain *string
	if err := json.Unmarshal(v, &plain); err != nil {
		return errf(codeConfig, "the value of the aes column %s is not text: %v", column, err)
	}
	if plain == nil {
		if indexed {
			out[index] = json.RawMessage("null")
		}
		return nil
	}
	stages := tb.Codecs[column]
	cell, err := orm.HostEncode(*plain, stages[slices.Index(stages, "aes"):], secret)
	if err != nil {
		return err
	}
	if b, ok := cell.([]byte); ok {
		cell = string(b)
	}
	text, ok := cell.(string)
	if !ok {
		return errf(codeInternal, "orm encodes the column %s to a %T", column, cell)
	}
	for _, col := range tb.Columns {
		if col.Name == column && col.Type.Kind == dbspec.TypeBytes {
			text = base64.StdEncoding.EncodeToString([]byte(text))
		}
	}
	stored, err := json.Marshal(text)
	if err != nil {
		return err
	}
	out[column] = stored
	if indexed {
		if keys.BlindIndex == "" {
			return errf(codeConfig, "the column %s has a blind index and the keyring has no blind_index key", column)
		}
		sum, err := orm.BlindIndex(*plain, keys.BlindIndex)
		if err != nil {
			return err
		}
		if out[index], err = json.Marshal(sum); err != nil {
			return err
		}
	}
	return nil
}
