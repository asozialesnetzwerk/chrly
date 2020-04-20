package di

import (
	"fmt"
	"net/url"
	"time"

	"github.com/goava/di"
	"github.com/spf13/viper"

	es "github.com/elyby/chrly/eventsubscribers"
	"github.com/elyby/chrly/http"
	"github.com/elyby/chrly/mojangtextures"
)

var mojangTextures = di.Options(
	di.Provide(newMojangTexturesProviderFactory),
	di.Provide(newMojangTexturesProvider),
	di.Provide(newMojangTexturesUuidsProviderFactory),
	di.Provide(newMojangTexturesBatchUUIDsProvider),
	di.Provide(newMojangTexturesRemoteUUIDsProvider),
	di.Provide(newMojangSignedTexturesProvider),
	di.Provide(newMojangTexturesStorageFactory),
)

func newMojangTexturesProviderFactory(
	container *di.Container,
	config *viper.Viper,
) (http.MojangTexturesProvider, error) {
	config.SetDefault("mojang_textures.enabled", true)
	if !config.GetBool("mojang_textures.enabled") {
		return &mojangtextures.NilProvider{}, nil
	}

	var provider *mojangtextures.Provider
	err := container.Resolve(&provider)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func newMojangTexturesProvider(
	emitter mojangtextures.Emitter,
	uuidsProvider mojangtextures.UUIDsProvider,
	texturesProvider mojangtextures.TexturesProvider,
	storage mojangtextures.Storage,
) *mojangtextures.Provider {
	return &mojangtextures.Provider{
		Emitter:          emitter,
		UUIDsProvider:    uuidsProvider,
		TexturesProvider: texturesProvider,
		Storage:          storage,
	}
}

func newMojangTexturesUuidsProviderFactory(
	config *viper.Viper,
	container *di.Container,
) (mojangtextures.UUIDsProvider, error) {
	preferredUuidsProvider := config.GetString("mojang_textures.uuids_provider.driver")
	if preferredUuidsProvider == "remote" {
		var provider *mojangtextures.RemoteApiUuidsProvider
		err := container.Resolve(&provider)

		return provider, err
	}

	var provider *mojangtextures.BatchUuidsProvider
	err := container.Resolve(&provider)

	return provider, err
}

func newMojangTexturesBatchUUIDsProvider(
	container *di.Container,
	config *viper.Viper,
	emitter mojangtextures.Emitter,
) (*mojangtextures.BatchUuidsProvider, error) {
	// TODO: remove usage of di.WithName() when https://github.com/goava/di/issues/11 will be resolved
	if err := container.Provide(func(emitter es.Subscriber, config *viper.Viper) *namedHealthChecker {
		config.SetDefault("healthcheck.mojang_batch_uuids_provider_cool_down_duration", time.Minute)

		return &namedHealthChecker{
			Name: "mojang-batch-uuids-provider-response",
			Checker: es.MojangBatchUuidsProviderResponseChecker(
				emitter,
				config.GetDuration("healthcheck.mojang_batch_uuids_provider_cool_down_duration"),
			),
		}
	}); err != nil {
		return nil, err
	}

	if err := container.Provide(func(emitter es.Subscriber, config *viper.Viper) *namedHealthChecker {
		config.SetDefault("healthcheck.mojang_batch_uuids_provider_queue_length_limit", 50)

		return &namedHealthChecker{
			Name: "mojang-batch-uuids-provider-queue-length",
			Checker: es.MojangBatchUuidsProviderQueueLengthChecker(
				emitter,
				config.GetInt("healthcheck.mojang_batch_uuids_provider_queue_length_limit"),
			),
		}
	}); err != nil {
		return nil, err
	}

	config.SetDefault("queue.loop_delay", 2*time.Second+500*time.Millisecond)
	config.SetDefault("queue.batch_size", 10)

	return &mojangtextures.BatchUuidsProvider{
		Emitter:        emitter,
		IterationDelay: config.GetDuration("queue.loop_delay"),
		IterationSize:  config.GetInt("queue.batch_size"),
	}, nil
}

func newMojangTexturesRemoteUUIDsProvider(
	config *viper.Viper,
	emitter mojangtextures.Emitter,
) (*mojangtextures.RemoteApiUuidsProvider, error) {
	remoteUrl, err := url.Parse(config.GetString("mojang_textures.uuids_provider.url"))
	if err != nil {
		return nil, fmt.Errorf("unable to parse remote url: %w", err)
	}

	return &mojangtextures.RemoteApiUuidsProvider{
		Emitter: emitter,
		Url:     *remoteUrl,
	}, nil
}

func newMojangSignedTexturesProvider(emitter mojangtextures.Emitter) mojangtextures.TexturesProvider {
	return &mojangtextures.MojangApiTexturesProvider{
		Emitter: emitter,
	}
}

func newMojangTexturesStorageFactory(
	uuidsStorage mojangtextures.UuidsStorage,
	texturesStorage mojangtextures.TexturesStorage,
) mojangtextures.Storage {
	return &mojangtextures.SeparatedStorage{
		UuidsStorage:    uuidsStorage,
		TexturesStorage: texturesStorage,
	}
}
