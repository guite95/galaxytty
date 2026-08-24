.PHONY: build test vet fmt helper-test helper-build helper-deploy
build:
	go build -o msg ./cmd/msg
test:
	go test ./...
vet:
	go vet ./...
fmt:
	gofmt -w cmd internal
helper-test:
	./scripts/test-helper.sh
helper-build:
	. ./scripts/android-env.sh; configure_android_sdk && ./android-helper/gradlew --project-dir android-helper :app:assembleDebug
helper-deploy:
	./scripts/deploy-helper.sh
