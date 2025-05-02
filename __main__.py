import pulumi
import pulumi_yandex as yandex

# 1. Получаем конфигурацию
config = pulumi.Config("yandex")
sa_key = config.require("serviceAccountKey")
folder_id = config.require("folderId")


# 2. Создаём провайдер
provider = yandex.Provider(
    "yandex",
    service_account_key_file=sa_key,
    folder_id=folder_id,
)

# 3. Создаём сеть и подсеть
network = yandex.VpcNetwork(
    "my-network",
    opts=pulumi.ResourceOptions(provider=provider),
)

subnet = yandex.VpcSubnet(
    "my-subnet",
    zone="ru-central1-a",
    network_id=network.id,
    v4_cidr_blocks=["192.168.10.0/24"],
    opts=pulumi.ResourceOptions(provider=provider),
)

# 4. Читаем файл напрямую через Python
ssh_public_key_path = "/home/pi/.ssh/tf-cloud-init.pub"  # <-- добавлено .pub

try:
    with open(ssh_public_key_path, "r") as f:
        public_key = f.read()
except Exception as e:
    raise RuntimeError(f"Не удалось прочитать SSH-ключ: {e}")

# 5. Создаём Compute Instance
instance = yandex.ComputeInstance(
    "my-instance",
    zone="ru-central1-a",
    resources=yandex.ComputeInstanceResourcesArgs(
        core_fraction=100,
        cores=2,
        memory=2,
    ),
    boot_disk=yandex.ComputeInstanceBootDiskArgs(
        initialize_params=yandex.ComputeInstanceBootDiskInitializeParamsArgs(
            image_id="fd85m9q2qspfnsv055rh",  # Ubuntu 22.04 LTS
        ),
    ),
    network_interfaces=[
        yandex.ComputeInstanceNetworkInterfaceArgs(
            subnet_id=subnet.id,
            nat=True,  # чтобы был публичный IP
        )
    ],
    metadata={
        "ssh-keys": pulumi.Output.format("ubuntu:{0}", public_key),
    },
    opts=pulumi.ResourceOptions(provider=provider),
)

# 6. Экспортируем публичный IP
public_ip = instance.network_interfaces[0].nat_ip_address

pulumi.export("instance_name", instance.name)
pulumi.export("public_ip", public_ip)
