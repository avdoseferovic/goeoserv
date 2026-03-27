package deep

import "github.com/ethanmoffat/eolib-go/v3/data"

type LookupType int

const (
	LookupTypeItem LookupType = 1
	LookupTypeNpc  LookupType = 2
)

type AdminInteractLookupRequest struct {
	LookupType LookupType
	ID         int
}

type DialogLine struct {
	Left  string
	Right string
}

func DeserializeAdminInteractTake(reader *data.EoReader) (*AdminInteractLookupRequest, error) {
	return &AdminInteractLookupRequest{
		LookupType: LookupType(reader.GetChar()),
		ID:         reader.GetShort(),
	}, nil
}

func SerializeAdminInteractAdd(lines []DialogLine) ([]byte, error) {
	w := data.NewEoWriter()
	for _, line := range lines {
		if err := w.AddString(line.Left); err != nil {
			return nil, err
		}
		if err := w.AddByte(0xFF); err != nil {
			return nil, err
		}
		if err := w.AddString(line.Right); err != nil {
			return nil, err
		}
		if err := w.AddByte(0xFF); err != nil {
			return nil, err
		}
	}
	return w.Array(), nil
}
