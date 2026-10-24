#define _GNU_SOURCE
/* Disposable qualification helper, never shipped as a privileged product tool. */
#include <sys/apparmor.h>
#include <linux/capability.h>
#include <sys/prctl.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sched.h>
#include <grp.h>
#include <fcntl.h>
#include <unistd.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

static void fail(const char *stage) { fprintf(stderr,"probe refused: %s\n",stage); _exit(1); }
static unsigned long number(const char *s) {
    char *end; errno=0; unsigned long n=strtoul(s,&end,10);
    if(errno || !*s || *end || n<1 || n>2147483647UL) fail("identity");
    return n;
}
int main(int argc,char **argv) {
    const char *opt=getenv("HOMENODE_CONFINEMENT_WRITE_PROBE");
    if(argc!=7 || geteuid()!=0 || !opt || strcmp(opt,"1")) fail("opt-in");
    uid_t uid=(uid_t)number(argv[2]); gid_t gid=(gid_t)number(argv[3]);
    if(uid<65536) fail("reserved UID");
    if(strncmp(argv[1],"libvirt-",8) || strlen(argv[1])!=44 || strncmp(argv[6],"/tmp/pulse-homenode-",20) || strchr(argv[6]+20,'/')) fail("generated targets");
    struct stat expected,joined;
    if(fstat(3,&expected) || setns(3,CLONE_NEWNS) || stat("/proc/self/ns/mnt",&joined) || expected.st_ino!=joined.st_ino || expected.st_dev!=joined.st_dev || close(3)) fail("mount namespace");
    if(aa_change_profile(argv[1])) fail("AppArmor transition");
    if(setgroups(0,NULL) || setresgid(gid,gid,gid) || setresuid(uid,uid,uid) || prctl(PR_SET_NO_NEW_PRIVS,1,0,0,0)) fail("drop authority");
    uid_t ru,eu,su;gid_t rg,eg,sg;
    struct __user_cap_header_struct header={.version=_LINUX_CAPABILITY_VERSION_3,.pid=0};
    struct __user_cap_data_struct caps[2];
    if(getresuid(&ru,&eu,&su) || getresgid(&rg,&eg,&sg) || ru!=uid || eu!=uid || su!=uid || rg!=gid || eg!=gid || sg!=gid || getgroups(0,NULL)!=0 || syscall(SYS_capget,&header,caps) || caps[0].effective || caps[1].effective || caps[0].permitted || caps[1].permitted) fail("credential verification");
    /* This profile denies fresh proc-attribute reads. The root parent observes
       our live label after credential drop, then explicitly admits the probes. */
    char acknowledged=0;
    if(write(4,"R",1)!=1 || close(4) || read(5,&acknowledged,1)!=1 || acknowledged!='A' || close(5)) fail("parent label acknowledgement");
    int fd=open(argv[4],O_RDWR|O_NOFOLLOW|O_CLOEXEC);
    if(fd<0 || close(fd)) fail("managed write-open positive control");
    fd=open(argv[5],O_WRONLY|O_NOFOLLOW|O_CLOEXEC);
    if(fd>=0){close(fd);fail("readonly negative control");}
    if(errno!=EACCES && errno!=EPERM) fail("readonly refusal classification");
    if(mkdir(argv[6],0700) && errno!=EEXIST) {
        if(errno!=EACCES && errno!=EPERM) fail("temporary directory classification");
        puts("{\"temporaryWrite\":false,\"directoryDevice\":0,\"directoryInode\":0}");return 0;
    }
    int dir=open(argv[6],O_RDONLY|O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC);
    struct stat metadata;
    if(dir<0 || fstat(dir,&metadata) || metadata.st_uid!=uid || (metadata.st_mode&07777)!=0700) fail("temporary directory admission");
    fd=openat(dir,"generated-probe",O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW|O_CLOEXEC,0600);
    int allowed=fd>=0;
    if(!allowed && errno!=EACCES && errno!=EPERM) fail("temporary write classification");
    if(allowed){
        const char payload[]="generated confinement probe\n";
        if(write(fd,payload,sizeof(payload)-1)!=(ssize_t)(sizeof(payload)-1) || fsync(fd) || close(fd) || unlinkat(dir,"generated-probe",0)) fail("generated write cleanup");
    }
    if(close(dir)) fail("directory close");
    /* The parent removes its host directory. Remove any namespace-private copy. */
    if(rmdir(argv[6]) && errno!=EACCES && errno!=EPERM) fail("temporary directory cleanup");
    printf("{\"temporaryWrite\":%s,\"directoryDevice\":%llu,\"directoryInode\":%llu}\n",allowed?"true":"false",(unsigned long long)metadata.st_dev,(unsigned long long)metadata.st_ino);
    return 0;
}
