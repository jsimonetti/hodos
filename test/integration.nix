{ nixpkgs ? <nixpkgs>
, pkgs ? import <nixpkgs> { inherit system; config = { }; }
, system ? builtins.currentSystem
} @args:

import "${nixpkgs}/nixos/tests/make-test-python.nix"
  ({ pkgs, ... }: {
    name = "hodos-integration";

    nodes.router = { config, lib, ... }: {

      imports = [
        ../module.nix
      ];

      services.hodos.enable = true;
      services.hodos.settings =
        {
          icmp_interval = "2s";
          icmp_timeout = "250ms";
          burst_size = 3;
          burst_interval = "10s";
          up_action = "";
          down_action = "";

          interfaces = [
            {
              name = "ens33";
              table = 2;
              minimum_up = 1;
              metric = 3;

              hosts = [
                {
                  name = "Google";
                  host = "8.8.8.8";
                }
                {
                  name = "Cloudflare";
                  host = "1.1.1.1";
                }
                {
                  name = "Cloudflare";
                  host = "2606:4700:4700::1111";
                }
              ];
            }
            {
              name = "ens34";
              table = 3;
              metric = 2;
              minimum_up = 1;

              hosts = [
                {
                  name = "Cloudflare";
                  host = "1.0.0.1";
                }
              ];
            }
          ];
        };
    };

    testScript = ''
      start_all()
      with subtest("Wait for Hodos and network ready"):
          # Ensure networking is online and hodos is ready.
          router.wait_for_unit("network-online.target")
          router.wait_for_unit("hodos.service")
    '';
  })
  args
