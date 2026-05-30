package mocks

import (
	"capsule-me/internal/domain/catalog"
	"errors"
	"strconv"
	"sync"
	"time"
)

type Cache struct {
	storage sync.Map
	ttl     time.Duration
	stop    chan struct{}
}

type session struct {
	value   interface{}
	created time.Time
}

// ttl в минутах
func NewCache(ttl int) *Cache {
	c := &Cache{
		ttl:  time.Duration(ttl) * time.Minute,
		stop: make(chan struct{}),
	}

	go c.gcLoop()

	return c
}

func (c *Cache) gc() {
	now := time.Now()

	for i := 0; i < 3; i++ {
		c.storage.Range(func(key, value interface{}) bool {
			if now.Sub(value.(session).created) > c.ttl {
				if k, _ := c.storage.Load(key); k != nil {
					c.storage.Delete(key)
				}
			}
			return true
		})
	}
}

func (c *Cache) gcLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.gc()
		case <-c.stop:
			return // предотрващение утечки горутин
		}
	}
}

func (c *Cache) saveToCache(key, value interface{}) {
	now := time.Now()
	c.storage.Store(key, session{value: value, created: now})
}

func (c *Cache) SaveIncomingFeature(chatID int64, f *catalog.IncomingFeature) error {
	if f == nil {
		return errors.New("nil incoming feature")
	}
	key := strconv.Itoa(int(chatID)) + ":feature"
	c.saveToCache(key, f)
	return nil
}

func (c *Cache) GetIncomingFeatureByID(chatID int64) (*catalog.IncomingFeature, error) {
	req := strconv.Itoa(int(chatID)) + ":feature"

	var result *catalog.IncomingFeature

	c.storage.Range(func(key, value interface{}) bool {
		if key == req {
			session := value.(session)
			result = session.value.(*catalog.IncomingFeature)
			// c.storage.Delete(key)
			return false
		}
		return true
	})

	if result != nil {
		return result, nil
	}

	return nil, errors.New("no feature in storage")
}

func (c *Cache) SaveCapsule(chatID int64, cap *catalog.Capsule) error {
	if cap == nil {
		return errors.New("nil capsule")
	}
	key := strconv.Itoa(int(chatID)) + ":capsule"
	c.saveToCache(key, cap)
	return nil
}

func (c *Cache) GetCapsuleByID(chatID int64) (*catalog.Capsule, error) {
	req := strconv.Itoa(int(chatID)) + ":capsule"

	var result *catalog.Capsule

	c.storage.Range(func(key, value interface{}) bool {
		if key == req {
			session := value.(session)
			result = session.value.(*catalog.Capsule)
			// c.storage.Delete(key)
			return false
		}
		return true
	})

	if result != nil {
		return result, nil
	}

	return nil, errors.New("no capsule in storage")
}

// она сохраняет лупки, они в entity как Outfits
func (c *Cache) SaveLooks(chatID int64, r *catalog.Recommendations) error {
	if r == nil {
		return errors.New("given nil recomendations")
	}

	if len(r.Outfits) == 0 {
		return errors.New("no looks to store")
	}
	key := strconv.Itoa(int(chatID)) + ":look:"
	i := 0
	for _, outfit := range r.Outfits {
		key += strconv.Itoa(i)
		session := session{
			value:   &outfit,
			created: time.Now(),
		}
		c.storage.Store(key, session)
		key = key[:len(key)-1]
		i++
	}
	return nil
}

func (c *Cache) GetLookByNumAndID(chatID int64, num int) (*catalog.Outfit, error) {
	req := strconv.Itoa(int(chatID)) + ":look:" + strconv.Itoa(num)

	var result *catalog.Outfit

	c.storage.Range(func(key, value interface{}) bool {
		if key == req {
			session := value.(session)
			result = session.value.(*catalog.Outfit)
			// c.storage.Delete(key)
			return false
		}
		return true
	})

	if result != nil {
		return result, nil
	}

	return nil, errors.New("no look in storage")
}
