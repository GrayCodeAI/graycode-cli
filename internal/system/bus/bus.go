package bus

import "context"

// Producer publishes messages to a topic/stream.
type Producer interface {
	Publish(ctx context.Context, data []byte) error
	Close() error
}

// Consumer receives messages from a topic/stream.
type Consumer interface {
	Listen(ctx context.Context, handler func(ctx context.Context, data []byte) error) error
	Close() error
}

// ChannelProducer is an in-process Producer backed by a Go channel.
type ChannelProducer struct {
	ch chan<- []byte
}

func NewChannelBus(bufSize int) (*ChannelProducer, *ChannelConsumer) {
	if bufSize <= 0 {
		bufSize = 256
	}
	ch := make(chan []byte, bufSize)
	return &ChannelProducer{ch: ch}, &ChannelConsumer{ch: ch}
}

func (p *ChannelProducer) Publish(_ context.Context, data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	p.ch <- cp
	return nil
}

func (p *ChannelProducer) Close() error {
	close(p.ch)
	return nil
}

// ChannelConsumer is an in-process Consumer backed by a Go channel.
type ChannelConsumer struct {
	ch <-chan []byte
}

func (c *ChannelConsumer) Listen(ctx context.Context, handler func(ctx context.Context, data []byte) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-c.ch:
			if !ok {
				return nil
			}
			if err := handler(ctx, msg); err != nil {
				continue
			}
		}
	}
}

func (c *ChannelConsumer) Close() error {
	return nil
}
