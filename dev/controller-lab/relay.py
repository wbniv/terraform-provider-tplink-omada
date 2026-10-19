"""TLS passthrough to one fixed lab destination; published on host loopback only."""

import select
import socket
import socketserver


class Relay(socketserver.BaseRequestHandler):
    def handle(self):
        try:
            with socket.create_connection(("controller", 8043), timeout=10) as remote:
                self.request.settimeout(30)
                remote.settimeout(30)
                peers = {self.request: remote, remote: self.request}
                while True:
                    ready, _, _ = select.select(list(peers), [], [], 60)
                    if not ready:
                        return
                    for source in ready:
                        data = source.recv(65536)
                        if not data:
                            return
                        peers[source].sendall(data)
        except (OSError, TimeoutError):
            # Startup/shutdown and clients closing connections are normal here.
            return


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True


if __name__ == "__main__":
    with Server(("0.0.0.0", 8043), Relay) as server:
        server.serve_forever()
