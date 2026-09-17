// Prints total system power in watts by reading SMC key PSTR (same source
// iStat Menus uses). Build: cc -O2 -framework IOKit smc-pstr.c -o smc-pstr
#include <IOKit/IOKitLib.h>
#include <stdio.h>
#include <string.h>
#include <stdint.h>
#include <stdlib.h>

typedef struct {
    uint32_t dataSize;
    uint32_t dataType;
    char     dataAttributes;
} KeyInfo;

typedef struct {
    uint32_t key;
    uint8_t  vers[8];
    uint8_t  pLimitData[16];
    KeyInfo  keyInfo;
    char     result;
    char     status;
    char     data8;
    uint32_t data32;
    uint8_t  bytes[32];
} SMCKeyData;

#define SMC_KERNEL_INDEX 2
#define SMC_CMD_READ_KEYINFO 9
#define SMC_CMD_READ_BYTES 5

static io_connect_t open_smc(void) {
    io_service_t svc = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("AppleSMC"));
    if (!svc) return 0;
    io_connect_t conn = 0;
    kern_return_t r = IOServiceOpen(svc, mach_task_self(), 0, &conn);
    IOObjectRelease(svc);
    return r == KERN_SUCCESS ? conn : 0;
}

static kern_return_t smc_call(io_connect_t conn, SMCKeyData *in, SMCKeyData *out) {
    size_t outSize = sizeof(SMCKeyData);
    return IOConnectCallStructMethod(conn, SMC_KERNEL_INDEX, in, sizeof(SMCKeyData), out, &outSize);
}

int main(int argc, char **argv) {
    const char *name = argc > 1 ? argv[1] : "PSTR";
    if (strlen(name) != 4) { fprintf(stderr, "key must be 4 chars\n"); return 2; }
    uint32_t key = ((uint32_t)name[0] << 24) | ((uint32_t)name[1] << 16) | ((uint32_t)name[2] << 8) | (uint32_t)name[3];

    io_connect_t conn = open_smc();
    if (!conn) { fprintf(stderr, "cannot open AppleSMC\n"); return 1; }

    SMCKeyData in, out;
    memset(&in, 0, sizeof(in));
    memset(&out, 0, sizeof(out));
    in.key = key;
    in.data8 = SMC_CMD_READ_KEYINFO;
    kern_return_t r = smc_call(conn, &in, &out);
    if (r != KERN_SUCCESS) { fprintf(stderr, "keyinfo failed: 0x%x\n", r); return 1; }

    uint32_t size = out.keyInfo.dataSize;
    uint32_t type = out.keyInfo.dataType;
    memset(&in, 0, sizeof(in));
    memset(&out, 0, sizeof(out));
    in.key = key;
    in.data8 = SMC_CMD_READ_BYTES;
    in.keyInfo.dataSize = size;
    r = smc_call(conn, &in, &out);
    IOServiceClose(conn);
    if (r != KERN_SUCCESS) { fprintf(stderr, "read failed: 0x%x\n", r); return 1; }
    if (type != 0x666c7420 /* "flt " */ || size != 4) { fprintf(stderr, "unexpected type/size\n"); return 1; }

    float watts;
    memcpy(&watts, out.bytes, 4);
    printf("%.2f\n", watts);
    return 0;
}
