# beevibe backend: SolidJS frontend -> Go binary (cgo + whisper.cpp) -> slim runtime.
#
# third_party/ and models/ are gitignored, so this image vendors whisper.cpp itself
# (same cmake invocation as `just whisper-lib`) and downloads the STT model.

# ---- stage 1: frontend -------------------------------------------------------

FROM node:26-slim AS web

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- stage 2: backend --------------------------------------------------------

FROM golang:1.27-bookworm AS backend

RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential cmake git curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 https://github.com/ggml-org/whisper.cpp /src/third_party/whisper.cpp \
    && cmake -S /src/third_party/whisper.cpp -B /src/third_party/whisper.cpp/build_go \
        -DCMAKE_BUILD_TYPE=Release \
        -DBUILD_SHARED_LIBS=OFF \
        -DWHISPER_BUILD_TESTS=OFF \
        -DWHISPER_BUILD_EXAMPLES=OFF \
        -DWHISPER_BUILD_SERVER=OFF \
    && cmake --build /src/third_party/whisper.cpp/build_go --target whisper -- -j"$(nproc)"

WORKDIR /src

# the `replace github.com/ggerganov/whisper.cpp/bindings/go => ./third_party/whisper.cpp/bindings/go`
# directive needs that directory to exist even to resolve the module graph; the
# clone above provides it.
COPY go.mod go.sum ./
RUN go mod download

# third_party/, models/, data/, bin/ and web/node_modules/ are gitignored host build
# artifacts; keep them out of the image so the in-image whisper.cpp build and the
# downloaded Go modules win.
COPY --exclude=third_party --exclude=models --exclude=data --exclude=bin --exclude=web/node_modules --exclude=.git --exclude=.jj . ./
COPY --from=web /src/web/dist ./web/dist

ENV CGO_ENABLED=1 \
    C_INCLUDE_PATH=/src/third_party/whisper.cpp/include:/src/third_party/whisper.cpp/ggml/include \
    LIBRARY_PATH=/src/third_party/whisper.cpp/build_go/src:/src/third_party/whisper.cpp/build_go/ggml/src

RUN go build -o /out/beevibe ./cmd/beevibe

RUN mkdir -p /out/models \
    && curl -fL -o /out/models/ggml-base.en.bin https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin

# ---- stage 3: runtime --------------------------------------------------------

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends libgomp1 libstdc++6 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=backend /out/beevibe /usr/local/bin/beevibe
COPY --from=backend /out/models /models

ENV DATA_DIR=/data \
    WHISPER_MODEL=/models/ggml-base.en.bin \
    PORT=8080

VOLUME /data
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/beevibe"]
