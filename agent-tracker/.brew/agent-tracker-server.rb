class AgentTrackerServer < Formula
  desc "Tmux-aware agent tracker server"
  homepage "https://github.com/david/agent-tracker"
  url "file:///var/folders/11/dhzcjp416tl1dkf16kxns3z00000gn/T/tmp.mJenIMNxo7/tracker-server.tar.gz"
  sha256 "cb24ca397f60e5209b79667e32dc0f98fd22a9ac85627d5eb79d0e9e8e75be55"
  version "local-20250917103405"

  def install
    bin.install "tracker-server"
  end

  service do
    run [opt_bin/"tracker-server"]
    keep_alive true
    working_dir var/"agent-tracker"
    log_path var/"log/agent-tracker-server.log"
    error_log_path var/"log/agent-tracker-server.log"
  end
end
