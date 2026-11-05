package httpx

import "encoding/json"

// Optional — поле PATCH, у которого «нет в запросе» и null означают разное:
// нет — не менять (Set = false), null — очистить (Set = true, Value = nil).
// Обычный *T эти случаи не различает. encoding/json вызывает UnmarshalJSON и для null,
// но не для отсутствующего поля — на этом всё и держится.
//
// Обобщение groups.OptionalString (тот пока остаётся как есть).
type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}
