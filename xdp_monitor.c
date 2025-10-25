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
    __u8  pkt_len_low;  // 包长度低8位
    __u16 first_u16;    // 前2个字节（可能是类型/长度）
    __u32 padding;      // 填充字段，保持结构对齐
};

struct flow_stats {
    __u64 packets;
    __u64 bytes;
    __u64 last_update; // 最后更新时间（纳秒），用于检测陈旧条目
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

    // 扫描前64字节寻找 IPv4 头（0x45 开头）
    // IPoIB 的头部大小不固定，需要动态查找
    #pragma unroll
    for (int offset = 0; offset < 64; offset += 2) {
        if (data + offset + sizeof(struct iphdr) <= data_end) {
            struct iphdr *test_ip = data + offset;
            unsigned char version_ihl = *(unsigned char *)test_ip;
            unsigned char version = version_ihl >> 4;
            unsigned char ihl = version_ihl & 0x0F;
            
            // 检查是否是有效的 IPv4 头
            if (version == 4 && ihl >= 5 && ihl <= 15) {
                // 进一步验证：检查总长度字段是否合理
                __u16 tot_len = __builtin_bswap16(test_ip->tot_len);
                __u32 pkt_len = data_end - data;
                
                // 总长度应该 <= 包长度，且 >= IP 头最小长度
                if (tot_len >= 20 && tot_len <= pkt_len && test_ip->protocol > 0) {
                    ip = test_ip;
                    goto parse_ip;
                }
            }
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
        key.padding = 0;

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

        // 获取当前时间戳（纳秒）
        __u64 current_time = bpf_ktime_get_ns();

        // 使用 IP 头中的 tot_len 字段统计真实的 IP 数据包大小
        // 这样可以排除 L2 封装开销（IPoIB 头部等）
        __u16 ip_total_len = __builtin_bswap16(ip->tot_len);
        __u64 bytes_to_count = ip_total_len;

        // 查找或创建流统计
        struct flow_stats *val = bpf_map_lookup_elem(&flows, &key);
        if (!val) {
            // 新流，直接创建
            struct flow_stats init = {1, bytes_to_count, current_time};
            bpf_map_update_elem(&flows, &key, &init, BPF_ANY);
        } else {
            // 累积统计
            __sync_fetch_and_add(&val->packets, 1);
            __sync_fetch_and_add(&val->bytes, bytes_to_count);
            val->last_update = current_time;
        }
        return XDP_PASS;
    }

handle_other:
    // 记录无法解析的包（用于调试）
    {
        struct flow_key key = {};
        key.src_ip = 0x00000000;
        key.dst_ip = 0x00000000;
        key.proto = 0;
        key.src_port = 0;
        key.dst_port = 0;
        key.pkt_len_low = (data_end - data) & 0xFF;  // 包长度低8位
        key.first_u16 = 0;
        key.padding = 0;
        
        // 读取前2个字节
        if (data + 2 <= data_end) {
            key.first_u16 = *((__u16 *)data);
        }

        __u64 current_time = bpf_ktime_get_ns();
        
        struct flow_stats *val = bpf_map_lookup_elem(&flows, &key);
        if (!val) {
            struct flow_stats init = {1, data_end - data, current_time};
            bpf_map_update_elem(&flows, &key, &init, BPF_ANY);
        } else {
            __sync_fetch_and_add(&val->packets, 1);
            __sync_fetch_and_add(&val->bytes, data_end - data);
            val->last_update = current_time;
        }
    }
    
    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
