package beater

import "fmt"

type Registry interface {
	ReadStreamInfo(*Stream) error
	WriteStreamInfo(*Stream) error
}

func generateKey(stream *Stream) string {
	return fmt.Sprintf("%v/%v", stream.Group.Name, stream.Name)
}
