FROM golang:1.25-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    tmux zsh jq python3 fzf git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/zsh testuser

WORKDIR /home/testuser/agent-tracker

# Source is bind-mounted at runtime; build happens in entrypoint
COPY test/docker/entrypoint.sh /entrypoint.sh
COPY test/docker/fake-agent.sh /home/testuser/fake-agent.sh
COPY test/docker/run-tests.sh /run-tests.sh
RUN chmod +x /entrypoint.sh /home/testuser/fake-agent.sh /run-tests.sh

ENV TMUX_BIN=/usr/bin/tmux
ENV GOPROXY=off

ENTRYPOINT ["/entrypoint.sh"]
