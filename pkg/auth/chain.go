package auth

import (
	"context"
	"errors"
)

type ForbiddenError struct {
	Message string
}

func (e *ForbiddenError) Error() string {
	return e.Message
}

type ResolverChain struct {
	resolvers []IdentityResolver
	cache     *IdentityCache
}

func NewResolverChain(cache *IdentityCache, resolvers ...IdentityResolver) *ResolverChain {
	return &ResolverChain{
		resolvers: resolvers,
		cache:     cache,
	}
}

func (c *ResolverChain) Resolve(ctx context.Context, token string) (*Identity, error) {
	if c.cache != nil {
		if id, ok := c.cache.Get(token); ok {
			return id, nil
		}
	}

	for _, r := range c.resolvers {
		id, err := r.Resolve(ctx, token)
		if err != nil {
			return nil, err // Reject
		}
		if id != nil {
			if c.cache != nil {
				c.cache.Put(token, id)
			}
			return id, nil // Success
		}
	}

	return nil, errors.New("no auth resolver matched token")
}
