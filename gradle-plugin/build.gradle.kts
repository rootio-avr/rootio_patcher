plugins {
    `java-gradle-plugin`
    `maven-publish`
}

group = "io.root"
version = "0.1.0"

java {
    sourceCompatibility = JavaVersion.VERSION_11
    targetCompatibility = JavaVersion.VERSION_11
}

gradlePlugin {
    plugins {
        create("rootIoPatcher") {
            id = "io.root.patcher"
            implementationClass = "io.root.patcher.RootIoPatcherPlugin"
        }
    }
}

dependencies {
    testImplementation(gradleTestKit())
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.0")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

tasks.test {
    useJUnitPlatform()
}
