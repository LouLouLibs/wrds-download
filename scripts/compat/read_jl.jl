# Read parquet files with Parquet2.jl and dump them in the harness' common form.
#
# usage: julia --project=scripts/compat scripts/compat/read_jl.jl OUT_DIR FILE.parquet...
# Writes OUT_DIR/parquet2jl/<case>.json; see compare.py for the value encoding.

using Dates, DecFP, JSON, Parquet2, Tables

const UNIX_DAYS = Dates.value(Date(1970, 1, 1))
const UNIX_MS = Dates.value(DateTime(1970, 1, 1))

kind(::Type{<:Bool}) = "bool"
kind(::Type{<:Integer}) = "int"
kind(::Type{Float32}) = "float32"
kind(::Type{Float64}) = "float"
kind(::Type{<:DecFP.DecimalFloatingPoint}) = "decimal"
kind(::Type{Date}) = "date"
kind(::Type{DateTime}) = "timestamp"   # Julia has no time-zone-aware DateTime
kind(::Type{Time}) = "time"
kind(::Type{<:AbstractString}) = "string"
kind(::Type) = "other"

encode(::Missing) = nothing
encode(x::Union{Bool,Integer,AbstractString}) = x
encode(x::AbstractFloat) = isnan(x) ? "nan" : isinf(x) ? (x > 0 ? "inf" : "-inf") : string(x)
encode(x::DecFP.DecimalFloatingPoint) = string(x)
encode(x::Date) = Dates.value(x) - UNIX_DAYS
encode(x::DateTime) = Int128(Dates.value(x) - UNIX_MS) * 1000  # ms → µs (±infinity overflows Int64)
encode(x::Time) = Dates.value(x) ÷ 1000                         # ns → µs
encode(x) = string(x)

function dump(path)
    ds = Parquet2.Dataset(path)
    cols = Tables.columns(ds)
    out = []
    for name in Tables.columnnames(cols)
        v = Tables.getcolumn(cols, name)
        T = nonmissingtype(eltype(v))
        push!(out, Dict("name" => String(name), "type" => string(T), "kind" => kind(T),
                        "values" => [encode(x) for x in v]))
    end
    nrows = isempty(out) ? 0 : length(first(out)["values"])
    Dict("rows" => nrows, "columns" => out)
end

function main(outdir, files)
    mkpath(joinpath(outdir, "parquet2jl"))
    for path in files
        case = splitext(basename(path))[1]
        res = try
            dump(path)
        catch e
            Dict("error" => sprint(showerror, e))
        end
        open(joinpath(outdir, "parquet2jl", case * ".json"), "w") do io
            JSON.print(io, res)
        end
    end
end

main(ARGS[1], ARGS[2:end])
