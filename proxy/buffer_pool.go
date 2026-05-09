package proxy

import bufferpool "github.com/libp2p/go-buffer-pool"

const proxyBufferSize = 32 * 1024

var defaultProxyBufferPool = newProxyBufferPool(proxyBufferSize)

type proxyBufferPool struct {
	size int
}

func newProxyBufferPool(size int) *proxyBufferPool {
	return &proxyBufferPool{size: size}
}

func (p *proxyBufferPool) Get() []byte {
	return bufferpool.Get(p.size)
}

func (p *proxyBufferPool) Put(buffer []byte) {
	if cap(buffer) < p.size {
		return
	}
	bufferpool.Put(buffer[:p.size])
}
