class Claudemap < Formula
  desc "Visualize and analyze Claude Code's CLAUDE.md context assembly"
  homepage "https://github.com/alonw0/claudemap"
  url "https://github.com/alonw0/claudemap/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER_SHA256"
  license "MIT"
  head "https://github.com/alonw0/claudemap.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X github.com/alonw0/claudemap/render.Version=#{version}"), "."
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/claudemap version")
    # Run check on an empty temp dir — should exit 0 with no findings
    (testpath/"CLAUDE.md").write("# Test\n")
    system "#{bin}/claudemap", "check"
  end
end
