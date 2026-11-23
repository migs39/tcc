// ot-helper: binário auxiliar que expõe os OTs 1-de-2 do libOTe (Masny–Rindal,
// MR19) por linha de comando, para o site (sender) e o programa local
// (receptor). D-002, D-003, D-008.
//
// As mensagens do protocolo trafegam como hexadecimal por stdin/stdout; quem
// chama decide como transportá-las. O OT 1-de-n de Naor–Pinkas é montado em Go
// sobre estes OTs (D-008).
//
// Modos:
//   ot-helper sender <m>
//       stdin:  requisição do receptor (hex)
//       (no MR19 a requisição não depende de nada do sender, por isso cabe
//       num único envio, seguido de uma única resposta)
//       stdout: linha 1 = resposta do sender (hex);
//               linhas 2..m+1 = "<k0> <k1>" (blocos de 128 bits, hex)
//       Aleatoriedade: semente do sistema (sysRandomSeed).
//
//   ot-helper requisicao <semente> <bits>
//       stdout: requisição do receptor (hex)
//
//   ot-helper extrair <semente> <bits>
//       stdin:  resposta do sender (hex)
//       stdout: m linhas, cada uma o bloco escolhido (hex)
//
// <semente> tem 128 bits (32 dígitos hex). <bits> é uma cadeia de '0'/'1' com
// as m escolhas. O receptor usa só o PRNG semeado com <semente> (D-001), então
// "requisicao" e "extrair" com os mesmos argumentos reproduzem o mesmo estado:
// "extrair" refaz a requisição internamente antes de ler a resposta.
//
// Nada aqui grava em disco nem fala com a rede.

#include <cstdint>
#include <iostream>
#include <stdexcept>
#include <string>
#include <vector>

#include "coproto/Socket/BufferingSocket.h"
#include "cryptoTools/Common/BitVector.h"
#include "cryptoTools/Crypto/PRNG.h"
#include "libOTe/Base/MasnyRindal.h"

using namespace osuCrypto;

namespace {

const std::uint64_t kMaxOTs = 1 << 16;

std::string paraHex(const std::uint8_t* d, std::size_t n) {
    static const char* dig = "0123456789abcdef";
    std::string s(2 * n, '0');
    for (std::size_t i = 0; i < n; ++i) {
        s[2 * i] = dig[d[i] >> 4];
        s[2 * i + 1] = dig[d[i] & 15];
    }
    return s;
}

int valorHex(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    throw std::runtime_error("hex inválido");
}

std::vector<std::uint8_t> deHex(const std::string& s) {
    if (s.size() % 2) throw std::runtime_error("hex com tamanho ímpar");
    std::vector<std::uint8_t> v(s.size() / 2);
    for (std::size_t i = 0; i < v.size(); ++i)
        v[i] = static_cast<std::uint8_t>(valorHex(s[2 * i]) << 4 | valorHex(s[2 * i + 1]));
    return v;
}

std::string blocoHex(const block& b) { return paraHex(b.data(), sizeof(block)); }

std::string lerLinha() {
    std::string s;
    std::getline(std::cin, s);
    while (!s.empty() && (s.back() == '\r' || s.back() == '\n' || s.back() == ' ')) s.pop_back();
    return s;
}

block semente(const std::string& h) {
    auto v = deHex(h);
    if (v.size() != sizeof(block)) throw std::runtime_error("semente precisa de 128 bits");
    block b;
    std::memcpy(b.data(), v.data(), sizeof(block));
    return b;
}

BitVector escolhas(const std::string& bits) {
    if (bits.empty() || bits.size() > kMaxOTs) throw std::runtime_error("quantidade de escolhas inválida");
    BitVector bv(bits.size());
    for (std::size_t i = 0; i < bits.size(); ++i) {
        if (bits[i] != '0' && bits[i] != '1') throw std::runtime_error("escolha deve ser 0 ou 1");
        bv[i] = bits[i] == '1';
    }
    return bv;
}

std::vector<std::uint8_t> saida(coproto::BufferingSocket& sock) {
    auto b = sock.getOutbound();
    if (!b) return {};
    return std::vector<std::uint8_t>(b->begin(), b->end());
}

int modoSender(std::uint64_t m) {
    if (m == 0 || m > kMaxOTs) throw std::runtime_error("m inválido");
    auto req = deHex(lerLinha());
    if (req.empty()) throw std::runtime_error("requisição vazia");

    PRNG prng(sysRandomSeed());
    std::vector<std::array<block, 2>> msgs(m);
    MasnyRindal ot;
    coproto::BufferingSocket sock;
    auto proto = ot.send(msgs, prng, sock) | macoro::make_eager();

    // No MR19 do libOTe o sender manda seu ponto antes de ler a requisição,
    // mas a requisição do receptor não depende dele: a resposta é esse ponto
    // seguido do que vier depois de processar a requisição.
    auto resp = saida(sock);
    sock.processInbound(req);
    auto resto = saida(sock);
    resp.insert(resp.end(), resto.begin(), resto.end());
    if (!proto.is_ready()) {
        sock.setError(coproto::code::closed);
        try { macoro::sync_wait(std::move(proto)); } catch (...) {}
        throw std::runtime_error("requisição incompleta ou inválida");
    }
    macoro::sync_wait(std::move(proto));

    std::cout << paraHex(resp.data(), resp.size()) << "\n";
    for (auto& p : msgs) std::cout << blocoHex(p[0]) << " " << blocoHex(p[1]) << "\n";
    return 0;
}

int modoReceptor(const std::string& sem, const std::string& bits, bool extrair) {
    auto ch = escolhas(bits);
    PRNG prng(semente(sem));
    std::vector<block> msgs(ch.size());
    MasnyRindal ot;
    coproto::BufferingSocket sock;
    auto proto = ot.receive(ch, msgs, prng, sock) | macoro::make_eager();

    auto req = saida(sock);
    if (req.empty()) throw std::runtime_error("receptor não gerou requisição");
    if (!extrair) {
        std::cout << paraHex(req.data(), req.size()) << std::endl;
        // Encerra o protocolo, que ficaria esperando a resposta do sender.
        sock.setError(coproto::code::closed);
        try { macoro::sync_wait(std::move(proto)); } catch (...) {}
        return 0;
    }

    auto resp = deHex(lerLinha());
    if (resp.empty()) throw std::runtime_error("resposta vazia");
    sock.processInbound(resp);
    if (!proto.is_ready()) {
        sock.setError(coproto::code::closed);
        try { macoro::sync_wait(std::move(proto)); } catch (...) {}
        throw std::runtime_error("resposta incompleta ou inválida");
    }
    macoro::sync_wait(std::move(proto));
    for (auto& b : msgs) std::cout << blocoHex(b) << "\n";
    return 0;
}

}  // namespace

int main(int argc, char** argv) {
    try {
        std::string modo = argc > 1 ? argv[1] : "";
        if (modo == "sender" && argc == 3) return modoSender(std::stoull(argv[2]));
        if (modo == "requisicao" && argc == 4) return modoReceptor(argv[2], argv[3], false);
        if (modo == "extrair" && argc == 4) return modoReceptor(argv[2], argv[3], true);
        std::cerr << "uso: ot-helper sender <m> | requisicao <semente> <bits> | extrair <semente> <bits>\n";
        return 2;
    } catch (const std::exception& e) {
        std::cerr << "ot-helper: " << e.what() << "\n";
        return 1;
    }
}
