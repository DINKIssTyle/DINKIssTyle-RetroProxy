// Created by DINKIssTyle on 2025. Copyright (C) 2025 DINKI'ssTyle. All rights reserved.

package proxy

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RendererPool manages a pool of Renderer instances for concurrent page rendering
type RendererPool struct {
	renderers chan *Renderer
	size      int
	mu        sync.RWMutex
	closed    bool
}

// NewRendererPool creates a new pool with the specified number of renderers
func NewRendererPool(size int) *RendererPool {
	if size <= 0 {
		size = 3 // Default pool size
	}

	pool := &RendererPool{
		renderers: make(chan *Renderer, size),
		size:      size,
	}

	// Pre-create renderers and add to pool
	for i := 0; i < size; i++ {
		pool.renderers <- NewRenderer()
	}

	return pool
}

// acquire gets a renderer from the pool with timeout or context cancellation
func (p *RendererPool) acquire(ctx context.Context, timeout time.Duration) (*Renderer, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, errors.New("pool is closed")
	}
	p.mu.RUnlock()

	select {
	case renderer := <-p.renderers:
		return renderer, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, ErrRendererBusy
	}
}

// release returns a renderer to the pool
func (p *RendererPool) release(renderer *Renderer) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		renderer.Close()
		return
	}
	p.mu.RUnlock()

	select {
	case p.renderers <- renderer:
		// Successfully returned to pool
	default:
		// Pool is full (shouldn't happen), close this renderer
		renderer.Close()
	}
}

// RenderPage renders a page using an available renderer from the pool
func (p *RendererPool) RenderPage(ctx context.Context, url string) (string, error) {
	renderer, err := p.acquire(ctx, 30*time.Second)
	if err != nil {
		return "", err
	}
	defer p.release(renderer)

	return renderer.RenderPage(ctx, url)
}

// RenderPageFull renders a page with CSS/JS intact (for Modern mode)
func (p *RendererPool) RenderPageFull(ctx context.Context, url string) (string, error) {
	renderer, err := p.acquire(ctx, 30*time.Second)
	if err != nil {
		return "", err
	}
	defer p.release(renderer)

	return renderer.RenderPageFull(ctx, url)
}

// RenderPageWithScreenshot renders a page and captures screenshot
func (p *RendererPool) RenderPageWithScreenshot(ctx context.Context, url string) (string, []byte, error) {
	renderer, err := p.acquire(ctx, 30*time.Second)
	if err != nil {
		return "", nil, err
	}
	defer p.release(renderer)

	return renderer.RenderPageWithScreenshot(ctx, url)
}

// CaptureScreenshotAndLinks captures full page screenshot with link coordinates
func (p *RendererPool) CaptureScreenshotAndLinks(ctx context.Context, url string) ([]byte, []LinkRect, []InputRect, string, error) {
	renderer, err := p.acquire(ctx, 60*time.Second) // Longer timeout for screenshots
	if err != nil {
		return nil, nil, nil, "", err
	}
	defer p.release(renderer)

	return renderer.CaptureScreenshotAndLinks(ctx, url)
}

// SubmitInput submits input to a form field and captures the result
func (p *RendererPool) SubmitInput(ctx context.Context, urlStr, xpath, text string, doEnter bool) ([]byte, []LinkRect, []InputRect, string, error) {
	renderer, err := p.acquire(ctx, 60*time.Second)
	if err != nil {
		return nil, nil, nil, "", err
	}
	defer p.release(renderer)

	return renderer.SubmitInput(ctx, urlStr, xpath, text, doEnter)
}

// RenderPageWithLayout extracts layout data for HTML 3.2 New mode
func (p *RendererPool) RenderPageWithLayout(ctx context.Context, url string) (*LayoutResult, error) {
	renderer, err := p.acquire(ctx, 60*time.Second)
	if err != nil {
		return nil, err
	}
	defer p.release(renderer)

	return renderer.RenderPageWithLayout(ctx, url)
}

// IsBusy returns true if all renderers in the pool are busy
func (p *RendererPool) IsBusy() bool {
	return len(p.renderers) == 0
}

// IsClosed reports whether the pool can no longer accept rendering work.
// A closed pool cannot be reopened; the server must create a new pool when it
// is started again.
func (p *RendererPool) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// Close closes all renderers in the pool
func (p *RendererPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// Close all renderers in the pool
	close(p.renderers)
	for renderer := range p.renderers {
		renderer.Close()
	}

	return nil
}
