package deep

import (
	"github.com/ethanmoffat/eolib-go/v3/data"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

const FamilyCaptcha = eonet.PacketFamily(249)

func SerializeCaptchaOpen(id, rewardExp int, captcha string) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddShort(id); err != nil {
		return nil, err
	}
	if err := w.AddThree(rewardExp); err != nil {
		return nil, err
	}
	if captcha != "" {
		if err := w.AddByte(0xFF); err != nil {
			return nil, err
		}
		if err := w.AddString(captcha); err != nil {
			return nil, err
		}
	}
	return w.Array(), nil
}

func SerializeCaptchaAgree(id int, captcha string) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddShort(id); err != nil {
		return nil, err
	}
	if err := w.AddString(captcha); err != nil {
		return nil, err
	}
	return w.Array(), nil
}

func SerializeCaptchaClose(experience int) ([]byte, error) {
	w := data.NewEoWriter()
	if err := w.AddInt(experience); err != nil {
		return nil, err
	}
	return w.Array(), nil
}

func DeserializeCaptchaRequest(reader *data.EoReader) int {
	return reader.GetShort()
}

func DeserializeCaptchaReply(reader *data.EoReader) (int, string, error) {
	id := reader.GetShort()
	value, err := reader.GetString()
	return id, value, err
}
