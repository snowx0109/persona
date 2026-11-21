@file:OptIn(kotlinx.serialization.ExperimentalSerializationApi::class)

package persona.core.contracts

import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerialInfo
import kotlinx.serialization.SerializationException
import kotlinx.serialization.descriptors.*
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.*

@SerialInfo
@Target(AnnotationTarget.PROPERTY)
annotation class WireEnum(vararg val values: String)

@SerialInfo
@Target(AnnotationTarget.PROPERTY)
annotation class WireTime(val unixSeconds: Boolean = false)

/** An optional field whose malformed value is safety-critical and must not be replaced by its default. */
@SerialInfo
@Target(AnnotationTarget.PROPERTY)
annotation class WireStrict

/** Apply D5 at field boundaries; a malformed optional field must not discard its siblings. */
abstract class WireSerializer<T>(private val generated: KSerializer<T>) : KSerializer<T> {
    override val descriptor: SerialDescriptor get() = generated.descriptor

    override fun deserialize(decoder: Decoder): T {
        val input = decoder as? JsonDecoder ?: throw SerializationException("Wire models require JSON")
        return input.json.decodeFromJsonElement(generated, clean(input.decodeJsonElement(), descriptor))
    }

    override fun serialize(encoder: Encoder, value: T) = generated.serialize(encoder, value)
}

private fun alternatives(descriptor: SerialDescriptor, index: Int): List<String> =
    descriptor.getElementAnnotations(index).filterIsInstance<JsonNames>().flatMap { it.names.asList() } +
        ProtocolCompatibility.fieldAliases(descriptor.getElementName(index))

private fun clean(value: JsonElement, descriptor: SerialDescriptor): JsonElement {
    if (value == JsonNull) {
        if (descriptor.isNullable) return value
        throw SerializationException("Unexpected null for ${descriptor.serialName}")
    }
    // Tool input and message metadata are opaque: never rewrite their keys or values.
    if (descriptor.serialName.startsWith("kotlinx.serialization.json.")) return value
    return when (descriptor.kind) {
        StructureKind.CLASS, StructureKind.OBJECT -> {
            val obj = value as? JsonObject ?: throw SerializationException("Expected object")
            buildJsonObject {
                for (index in 0 until descriptor.elementsCount) {
                    val name = descriptor.getElementName(index)
                    // A key renamed on the wire keeps its old name as an alternative (@JsonNames: `plugin` before #1844).
                    val field = obj[name] ?: alternatives(descriptor, index).firstNotNullOfOrNull { obj[it] } ?: continue
                    try {
                        val child = descriptor.getElementDescriptor(index)
                        var cleaned = clean(field, child)
                        for (annotation in descriptor.getElementAnnotations(index)) {
                            when (annotation) {
                                is WireEnum -> if (cleaned != JsonNull && cleaned.jsonPrimitive.content !in annotation.values) {
                                    cleaned = JsonPrimitive("unknown")
                                }
                                is WireTime -> cleaned = if (annotation.unixSeconds) {
                                    cleaned.takeIf { (it as? JsonPrimitive)?.doubleOrNull?.let { seconds -> seconds >= 946684800.0 } == true } ?: JsonNull
                                } else {
                                    parseTime((cleaned as? JsonPrimitive)?.contentOrNull)?.let { JsonPrimitive(it.toString()) } ?: JsonNull
                                }
                            }
                        }
                        if (cleaned == JsonNull && !child.isNullable && descriptor.isElementOptional(index)) continue
                        put(name, cleaned)
                    } catch (error: SerializationException) {
                        val strict = descriptor.getElementAnnotations(index).any { it is WireStrict }
                        if (!descriptor.isElementOptional(index) || strict) throw error
                    }
                }
                for (index in 0 until descriptor.elementsCount) {
                    if (!descriptor.isElementOptional(index) && descriptor.getElementName(index) !in obj &&
                        alternatives(descriptor, index).none { it in obj }) {
                        throw SerializationException("Missing ${descriptor.getElementName(index)}")
                    }
                }
            }
        }
        StructureKind.LIST -> JsonArray((value as? JsonArray ?: throw SerializationException("Expected array"))
            .map { clean(it, descriptor.getElementDescriptor(0)) })
        StructureKind.MAP -> JsonObject((value as? JsonObject ?: throw SerializationException("Expected map"))
            .mapValues { clean(it.value, descriptor.getElementDescriptor(1)) })
        is PrimitiveKind -> {
            val primitive = value as? JsonPrimitive ?: throw SerializationException("Expected primitive")
            val valid = when (descriptor.kind) {
                PrimitiveKind.STRING, PrimitiveKind.CHAR -> primitive.isString
                PrimitiveKind.BOOLEAN -> !primitive.isString && primitive.booleanOrNull != null
                PrimitiveKind.INT -> !primitive.isString && primitive.intOrNull != null
                PrimitiveKind.LONG -> !primitive.isString && primitive.longOrNull != null
                PrimitiveKind.DOUBLE, PrimitiveKind.FLOAT -> !primitive.isString && primitive.doubleOrNull?.isFinite() == true
                else -> true
            }
            if (!valid) throw SerializationException("Invalid ${descriptor.serialName}")
            value
        }
        else -> value
    }
}
