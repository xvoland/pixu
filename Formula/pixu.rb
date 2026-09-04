class Pixu < Formula
  desc "Terminal image viewer - render images directly in your terminal"
  homepage "https://github.com/xvoland/pixu"
  url "https://github.com/xvoland/pixu/archive/refs/tags/v1.3.5.tar.gz"
  sha256 "52f71d2937b0ec1115143b1277cd27c13593006fe3b81cb151c2452793524221"
  version "1.3.5"
  license "Apache-2.0"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version} -X main.buildSource=#{version}"
    system "go", "build", "-ldflags", ldflags, "-o", bin/"pixu", "."
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/pixu --version")
  end
end
