{ pkgs ? import <nixpkgs> { }
,
}:
let
  version = "0.0.1";
  vendorSha = "sha256-/Dr3qxpSFNKwNJ1LUty42Yq55ZX8kikqMOdR0rdmzwI=";
  buildGoModule = if pkgs.lib.versionOlder pkgs.go.version "1.18" then pkgs.buildGo118Module else pkgs.buildGoModule;
in
buildGoModule {
  pname = "hodos";
  version = version;
  src = ./.;
  vendorSha256 = vendorSha;

  preBuild = ''
    buildFlagsArray=(
      -ldflags="
        -X github.com/jsimonetti/hodos/internal/build.linkVersion=v${version}
      "
    )
  '';

  subPackages = [ "./cmd/hodos" ];
}
