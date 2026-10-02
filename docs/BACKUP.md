# External backup in HomeNode

This guide covers the implemented backup interface. HomeNode is still under
development: physical-drive, live-VM, replacement-host and release acceptance
remain incomplete. Do not treat a successful development test as proof that your
applications can be recovered.

## Before starting

A trusted installation must configure the isolated backup worker and register the
external drive's filesystem UUID and encrypted restic repository identity. The
browser cannot register, format or adopt a drive in this build. If the interface
says execution is not configured, installation configuration is required first.

Keep the repository password and host recovery material somewhere other than the
server and backup drive. A connected writable backup drive remains vulnerable to
host compromise. HomeNode does not provide unattended backups or immutable
connected-drive protection.

## Start a backup

1. Attach the registered drive and open **External backup** as the administrator.
2. Select **Check backup availability**. Availability describes sampled job
   admission, not drive presence or repository health. If paused, finish or recover
   the current maintenance operation. If unavailable, recheck before entering a
   password; the interface has not established why admission is unavailable.
3. Enter the repository password and select **Verify passkey and start backup**.
   Fresh passkey verification binds approval to this repository and request.
   The password field clears immediately, including when verification is cancelled.
4. A started job means the controller accepted work. Select **Refresh backup
   status** to inspect its durable outcome. Do not equate acceptance with a
   completed backup.

Checking availability clears any entered password. A paused, unavailable or failed
check hides credential entry. When admission becomes available again, checking
restores an empty field. The server always rechecks actual admission at job start.

Backup closes new-work admission, drains finite work and stops persistent
workloads before copying stable data. The isolated worker verifies the exact
registered mount and repository before acquiring runtime maintenance authority.
A missing or mismatched drive is rejected; it must not become a directory on the
host disk. Safely disconnect the drive after the operation completes and workload
restoration is confirmed.

## Read status and recover workloads

**Last acknowledged publication** records durable publication evidence. It is
separate from repository validation and a successful application restore test.
An earlier valid publication stays visible when a later attempt is refused.

**Uncertain** means completion or publication has not been established. New work
remains paused. Do not resubmit a backup to guess that the old worker stopped.
Explicit uncertain-worker reconciliation remains incomplete in this build.

**Repository refused before runtime acquisition** means the authenticated worker
reported that it stopped at repository admission. HomeNode attempts workload
restoration without claiming a new snapshot or releasing authority it never
acquired. If restoration fails, admission remains closed.

When **Verify passkey and resume workloads** appears, HomeNode has qualified an
owned stopped-worker recovery checkpoint. Verify again to retry workload
restoration. This action resumes workloads; it does not restore backup data or
repeat publication. A started recovery remains distinct from successful workload
restoration; refresh status afterward. If no recovery action is offered, do not
manually remove maintenance records or infer recovery from an empty runtime token.

## Reminders and restore limitations

The administrator can save a reminder interval of 1–90 days; the default is seven.
Reminders appear when status is checked and use the last acknowledged publication.
They do not schedule backups or send external notifications.

The repository restore components check manifest compatibility, payload integrity,
removed management authority and recovered ext4 filesystems without repair. Disk
checks do not establish application health or grant runtime start authority. The
owner-facing backup-data restore and replacement-host recovery workflow is not
available yet. New identity, recovery epoch, trust reconstruction, fresh device
and passkey enrollment, disconnected disk installation and application health
checks remain acceptance work. Repository integrity checks alone cannot establish
application recoverability.
