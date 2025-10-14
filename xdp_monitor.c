//go:build ignore
// +build ignore
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>

// RoCE v2 使用的 UDP 端口
#define ROCE_V2_PORT 4791

// Linux SLL (cooked) header size
#define SLL_HDR_LEN 16

// IPoIB 头部大小 (4 bytes hardware header)
#define IPOIB_HEADER_LEN 4

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
    struct iphdr *ip = 0;
    int ip_offset = 0;

    // 方法1: 尝试 IPoIB (4 字节硬件头)
    if (data + IPOIB_HEADER_LEN + sizeof(struct iphdr) <= data_end) {
        struct iphdr *test_ip = data + IPOIB_HEADER_LEN;
        if (test_ip->version == 4) {
            ip_offset = IPOIB_HEADER_LEN;
            ip = test_ip;
            goto parse_ip;
        }
    }

    // 方法2: 尝试标准以太网头部
    if (data + sizeof(struct ethhdr) <= data_end) {
        struct ethhdr *eth = data;
        if (eth->h_proto == __constant_htons(ETH_P_IP)) {
            ip_offset = sizeof(struct ethhdr);
            ip = data + ip_offset;
            goto parse_ip;
        }
    }
    
    // 方法3: 尝试 SLL 格式（某些 IPoIB）
    if (data + SLL_HDR_LEN + sizeof(struct iphdr) <= data_end) {
        struct iphdr *test_ip = data + SLL_HDR_LEN;
        if (test_ip->version == 4) {
            ip_offset = SLL_HDR_LEN;
            ip = test_ip;
            goto parse_ip;
        }
    }
    
    // 如果找不到 IP 头，跳到其他协议处理
    goto handle_other;

parse_ip:
    // 处理 IP 数据包
    if ((void *)(ip + 1) <= data_end) {
        // 验证是否是 IPv4
        if (ip->version != 4)
            goto handle_other;

        struct flow_key key = {};
        key.src_ip = ip->saddr;
        key.dst_ip = ip->daddr;
        key.proto = ip->protocol;

        if (ip->protocol == IPPROTO_TCP || ip->protocol == IPPROTO_UDP) {
            struct tcphdr *tcp = (void *)ip + ip->ihl * 4;
            if ((void *)(tcp + 1) > data_end)
                goto handle_other;
            key.src_port = tcp->source;
            key.dst_port = tcp->dest;
            
            // 检测 RoCE v2 流量 (UDP port 4791)
            if (ip->protocol == IPPROTO_UDP && 
                (__builtin_bswap16(key.dst_port) == ROCE_V2_PORT || 
                 __builtin_bswap16(key.src_port) == ROCE_V2_PORT)) {
                // 标记为 RoCE v2 流量
                key.proto = 0xFE; // 使用特殊标记表示 RoCE v2
            }
        }

        struct flow_stats *val = bpf_map_lookup_elem(&flows, &key);
        if (!val) {
            struct flow_stats init = {1, data_end - data};
            bpf_map_update_elem(&flows, &key, &init, BPF_ANY);
        } else {
            __sync_fetch_and_add(&val->packets, 1);
            __sync_fetch_and_add(&val->bytes, data_end - data);
        }
        return XDP_PASS;
    }

handle_other:
    // 处理 InfiniBand 和 RoCE v1 协议
    if (data + sizeof(struct ethhdr) <= data_end) {
        struct ethhdr *eth = data;
        if (eth->h_proto == __constant_htons(0x8915) ||  // ETH_P_IBOE (RoCE v1)
            eth->h_proto == __constant_htons(0x8914)) {  // ETH_P_IB
            // InfiniBand/RoCE v1/RDMA 数据包处理
            struct flow_key key = {};
            key.src_ip = 0x01000000;  // 标记为 InfiniBand/RoCE 流量
            key.dst_ip = 0x02000000;  // 标记为 InfiniBand/RoCE 流量
            key.proto = eth->h_proto; // 使用实际的协议类型
            key.src_port = 0;
            key.dst_port = 0;

            struct flow_stats *val = bpf_map_lookup_elem(&flows, &key);
            if (!val) {
                struct flow_stats init = {1, data_end - data};
                bpf_map_update_elem(&flows, &key, &init, BPF_ANY);
            } else {
                __sync_fetch_and_add(&val->packets, 1);
                __sync_fetch_and_add(&val->bytes, data_end - data);
            }
            return XDP_PASS;
        }
        // 处理其他协议（包括可能的 RDMA over Ethernet）
        else {
            // 通用流量统计 - 捕获所有其他协议
            struct flow_key key = {};
            key.src_ip = 0x00000000;
            key.dst_ip = 0x00000000;
            key.proto = eth->h_proto;
            key.src_port = 0;
            key.dst_port = 0;

            struct flow_stats *val = bpf_map_lookup_elem(&flows, &key);
            if (!val) {
                struct flow_stats init = {1, data_end - data};
                bpf_map_update_elem(&flows, &key, &init, BPF_ANY);
            } else {
                __sync_fetch_and_add(&val->packets, 1);
                __sync_fetch_and_add(&val->bytes, data_end - data);
            }
            return XDP_PASS;
        }
    }
    
    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
