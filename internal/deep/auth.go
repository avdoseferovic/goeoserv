package deep

import "github.com/ethanmoffat/eolib-go/v3/data"

const (
	ActionConfig = 220
)

type AccountAcceptRequest struct {
	SequenceNumber int
	AccountName    string
	EmailAddress   string
}

type LoginCreateRequest struct {
	AccountName string
}

type LoginAgreeRequest struct {
	AccountName string
	Pin         string
	Password    string
}

func SerializeLoginConfig(maxSkins, maxHairStyles, maxCharacterName int) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddShort(maxSkins); err != nil {
		return nil, err
	}
	if err := w.AddShort(maxHairStyles); err != nil {
		return nil, err
	}
	if err := w.AddChar(maxCharacterName); err != nil {
		return nil, err
	}
	return w.Array(), nil
}

func SerializeAccountConfig(delayTime int, emailValidation bool) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddShort(delayTime); err != nil {
		return nil, err
	}
	flag := 0
	if emailValidation {
		flag = 1
	}
	if err := w.AddChar(flag); err != nil {
		return nil, err
	}
	return w.Array(), nil
}

func DeserializeAccountAccept(reader *data.EoReader) (*AccountAcceptRequest, error) {
	oldChunked := reader.IsChunked()
	defer reader.SetIsChunked(oldChunked)
	reader.SetIsChunked(true)
	req := &AccountAcceptRequest{SequenceNumber: reader.GetShort()}
	if err := reader.NextChunk(); err != nil {
		return nil, err
	}
	name, err := reader.GetString()
	if err != nil {
		return nil, err
	}
	req.AccountName = name
	if err := reader.NextChunk(); err != nil {
		return nil, err
	}
	email, err := reader.GetString()
	if err != nil {
		return nil, err
	}
	req.EmailAddress = email
	return req, nil
}

func SerializeAccountAcceptReply(code int) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddShort(code); err != nil {
		return nil, err
	}
	return w.Array(), nil
}

func SerializeShortCode(code int) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddShort(code); err != nil {
		return nil, err
	}
	return w.Array(), nil
}

func SerializeLoginCreateReply(code int, email string) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddShort(code); err != nil {
		return nil, err
	}
	if email != "" {
		if err := w.AddByte(0xFF); err != nil {
			return nil, err
		}
		if err := w.AddString(email); err != nil {
			return nil, err
		}
	}
	return w.Array(), nil
}

func DeserializeLoginCreate(reader *data.EoReader) (*LoginCreateRequest, error) {
	name, err := reader.GetString()
	if err != nil {
		return nil, err
	}
	return &LoginCreateRequest{AccountName: name}, nil
}

func DeserializeLoginAccept(reader *data.EoReader) (string, error) {
	return reader.GetString()
}

func DeserializeLoginAgree(reader *data.EoReader) (*LoginAgreeRequest, error) {
	oldChunked := reader.IsChunked()
	defer reader.SetIsChunked(oldChunked)
	reader.SetIsChunked(true)
	name, err := reader.GetString()
	if err != nil {
		return nil, err
	}
	if err := reader.NextChunk(); err != nil {
		return nil, err
	}
	pin, err := reader.GetString()
	if err != nil {
		return nil, err
	}
	if err := reader.NextChunk(); err != nil {
		return nil, err
	}
	password, err := reader.GetString()
	if err != nil {
		return nil, err
	}
	return &LoginAgreeRequest{AccountName: name, Pin: pin, Password: password}, nil
}
