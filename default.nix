{buildGoModule}:
buildGoModule {
  pname = "go-grep-cache";
  version = "0.0.1";

  src = ./.;

  vendorHash = "sha256-gJ0+2ZSng9/6hQ6hUqcNnwwaWSBWoXP9DgaNtq/lWXQ=";

  ldflags = ["-s" "-w"];
}
