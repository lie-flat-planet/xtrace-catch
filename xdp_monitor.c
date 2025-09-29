//go:build ignore
// +build ignore
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>

struct flow_key {
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8  proto;
};

struct flow_stats {
    __u64 packets;
    __u64 bytes;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, struct flow_key);
    __type(value, struct flow_stats);
} flows SEC(".maps");

SEC("xdp")
int xdp_monitor(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;

    if (eth->h_proto == __constant_htons(ETH_P_IP)) {
        struct iphdr *ip = (void *)eth + sizeof(*eth);
        if ((void *)(ip + 1) > data_end)
            return XDP_PASS;

        struct flow_key key = {};
        key.src_ip = ip->saddr;
        key.dst_ip = ip->daddr;
        key.proto = ip->protocol;

        if (ip->protocol == IPPROTO_TCP || ip->protocol == IPPROTO_UDP) {
            struct tcphdr *tcp = (void *)ip + ip->ihl * 4;
            if ((void *)(tcp + 1) > data_end)
                return XDP_PASS;
            key.src_port = tcp->source;
            key.dst_port = tcp->dest;
        }

        struct flow_stats *val = bpf_map_lookup_elem(&flows, &key);
        if (!val) {
            struct flow_stats init = {1, data_end - data};
            bpf_map_update_elem(&flows, &key, &init, BPF_ANY);
        } else {
            __sync_fetch_and_add(&val->packets, 1);
            __sync_fetch_and_add(&val->bytes, data_end - data);
        }
    }
    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
